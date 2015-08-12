package renter

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

type uploadPiece struct {
	data       []byte
	chunkIndex uint64
	pieceIndex uint64
}

// An uploader uploads pieces to a host. This interface exists to facilitate
// easy testing.
// TODO: once fileContracts are stored in the renter, have these return IDs?
type uploader interface {
	// connect initiates the connection to the uploader.
	connect() (fileContract, error)

	// addPiece uploads a piece to the uploader.
	addPiece(uploadPiece) (fileContract, error)
}

// worker uploads pieces to a host as directed by reqChan. It sends the
// updated fileContract down respChan.
func (f *file) worker(host uploader, reqChan chan uploadPiece, respChan chan *fileContract) {
	// TODO: move connect outside worker
	_, err := host.connect()
	if err != nil {
		respChan <- nil
		return
	}
	for req := range reqChan {
		contract, err := host.addPiece(req)
		if err != nil {
			respChan <- nil
			return // this host is now dead to us; upload will use a new one
		}
		respChan <- &contract
	}
}

// upload reads chunks from r and uploads them to hosts. It spawns a worker
// for each host, and instructs them to upload pieces of each chunk.
func (f *file) upload(r io.Reader, hosts []uploader) error {
	// create request/response channels and spawn workers
	reqChans := make([]chan uploadPiece, len(hosts))
	respChans := make([]chan *fileContract, len(hosts))
	for i, h := range hosts {
		reqChans[i] = make(chan uploadPiece)
		respChans[i] = make(chan *fileContract)
		go f.worker(h, reqChans[i], respChans[i])
	}

	chunk := make([]byte, f.chunkSize())
	for i := uint64(0); ; i++ {
		// read next chunk
		n, err := io.ReadFull(r, chunk)
		if err == io.EOF {
			break
		} else if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
		// encode
		pieces, err := f.ecc.Encode(chunk)
		if err != nil {
			return err
		}
		// send upload requests to workers
		for j, ch := range reqChans {
			ch <- uploadPiece{pieces[j], i, uint64(j)}
		}
		// read upload responses from workers
		for _, ch := range respChans {
			contract := <-ch
			if contract == nil {
				// choose new host somehow
				//go f.worker(newhost, reqChans[j], respChans[j])
				continue
			}
			f.Contracts[contract.IP] = *contract
		}
		f.bytesUploaded += uint64(n) // TODO: move inside workers
		f.chunksUploaded++
	}
	return nil
}

// checkWalletBalance looks at an upload and determines if there is enough
// money in the wallet to support such an upload. An error is returned if it is
// determined that there is not enough money.
func (r *Renter) checkWalletBalance(up modules.FileUploadParams) error {
	// Get the size of the file.
	fileInfo, err := os.Stat(up.Filename)
	if err != nil {
		return err
	}
	curSize := types.NewCurrency64(uint64(fileInfo.Size()))

	var averagePrice types.Currency
	sampleSize := up.ECC.NumPieces() * 3 / 2
	hosts := r.hostDB.RandomHosts(sampleSize)
	for _, host := range hosts {
		averagePrice = averagePrice.Add(host.Price)
	}
	if len(hosts) == 0 {
		return errors.New("no hosts!")
	}
	averagePrice = averagePrice.Div(types.NewCurrency64(uint64(len(hosts))))
	estimatedCost := averagePrice.Mul(types.NewCurrency64(uint64(up.Duration))).Mul(curSize)
	bufferedCost := estimatedCost.Mul(types.NewCurrency64(2))

	siacoinBalance, _, _ := r.wallet.ConfirmedBalance()
	if bufferedCost.Cmp(siacoinBalance) > 0 {
		return errors.New("insufficient balance for upload")
	}
	return nil
}

// Upload takes an upload parameters, which contain a file to upload, and then
// creates a redundant copy of the file on the Sia network.
func (r *Renter) Upload(up modules.FileUploadParams) error {
	// TODO: This type of restriction is something that should be handled by
	// the frontend, not the backend.
	if filepath.Ext(up.Filename) != filepath.Ext(up.Nickname) {
		return errors.New("nickname and file name must have the same extension")
	}

	// Open the file.
	handle, err := os.Open(up.Filename)
	if err != nil {
		return err
	}

	err = r.checkWalletBalance(up)
	if err != nil {
		return err
	}

	// Check for a nickname conflict.
	lockID := r.mu.RLock()
	_, exists := r.files[up.Nickname]
	r.mu.RUnlock(lockID)
	if exists {
		return errors.New("file with that nickname already exists")
	}

	// Check that the file is less than 5 GiB.
	fileInfo, err := handle.Stat()
	if err != nil {
		return err
	}
	// NOTE: The upload max of 5 GiB is temporary and therefore does not have
	// a constant. This should be removed once micropayments + upload resuming
	// are in place. 5 GiB is chosen to prevent confusion - on anybody's
	// machine any file appearing to be under 5 GB will be below the hard
	// limit.
	if fileInfo.Size() > 5*1024*1024*1024 {
		return errors.New("cannot upload a file larger than 5 GB")
	}

	// Check that the hostdb is sufficiently large to support an upload.
	// TODO: ActiveHosts needs to only report hosts >= v0.4
	if len(r.hostDB.ActiveHosts()) < up.ECC.NumPieces() {
		return errors.New("not enough hosts on the network to upload a file")
	}

	// Create file object.
	f := newFile(up.ECC, up.PieceSize, uint64(fileInfo.Size()))

	// Add file to renter.
	lockID = r.mu.Lock()
	r.files[up.Nickname] = f
	r.save()
	r.mu.Unlock(lockID)

	// Upload to hosts in parallel.
	var hosts []uploader
	// TODO: hostDB needs to provide uploaders, or we need to convert them somehow
	for range r.hostDB.RandomHosts(up.ECC.NumPieces()) {
		hosts = append(hosts, nil)
	}
	err = f.upload(handle, hosts)
	if err != nil {
		// Upload failed; remove the file object.
		lockID = r.mu.Lock()
		delete(r.files, up.Nickname)
		r.save()
		r.mu.Unlock(lockID)
		return errors.New("failed to upload any file pieces")
	}

	return nil
}
