package renter

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	defaultDuration     = 6000 // Duration that hosts will hold onto the file
	defaultDataPieces   = 2    // Data pieces per erasure-coded chunk
	defaultParityPieces = 10   // Parity pieces per erasure-coded chunk

	// piece sizes
	// NOTE: The encryption overhead is subtracted so that encrypted piece
	// will always be a multiple of 64 (i.e. crypto.SegmentSize). Without this
	// property, revisions break the file's Merkle root.
	defaultPieceSize = 1<<22 - crypto.TwofishOverhead // 4 MiB
	smallPieceSize   = 1<<16 - crypto.TwofishOverhead // 64 KiB
)

type uploadPiece struct {
	data       []byte
	chunkIndex uint64
	pieceIndex uint64
}

// An uploader uploads pieces to a host. This interface exists to facilitate
// easy testing.
type uploader interface {
	// addPiece uploads a piece to the uploader.
	addPiece(uploadPiece) error

	// fileContract returns the fileContract containing the metadata of all
	// previously added pieces.
	fileContract() fileContract

	// addr returns the IP address of the uploader.
	addr() modules.NetAddress
}

// upload reads chunks from r and uploads them to hosts. It spawns a worker
// for each host, and instructs them to upload pieces of each chunk.
func (f *file) upload(r io.Reader, hosts []uploader) error {
	// encode and upload each chunk
	var wg sync.WaitGroup
	for i := uint64(0); ; i++ {
		// read next chunk
		chunk := make([]byte, f.chunkSize())
		_, err := io.ReadFull(r, chunk)
		if err == io.EOF {
			break
		} else if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
		// encode
		pieces, err := f.erasureCode.Encode(chunk)
		if err != nil {
			return err
		}
		// upload pieces, split evenly among hosts
		wg.Add(len(pieces))
		for j, data := range pieces {
			host := hosts[j%len(hosts)]
			up := uploadPiece{data, i, uint64(j)}
			go func(host uploader, up uploadPiece) {
				_ = host.addPiece(up)
				wg.Done()
			}(host, up)
		}
		wg.Wait()

		// update contracts
		for _, h := range hosts {
			contract := h.fileContract()
			f.contracts[contract.ID] = contract
		}
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
	sampleSize := up.ErasureCode.NumPieces() * 3 / 2
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
	// Open the file.
	handle, err := os.Open(up.Filename)
	if err != nil {
		return err
	}
	defer handle.Close()

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

	// Fill in any missing upload params with sensible defaults.
	if up.Duration == 0 {
		up.Duration = defaultDuration
	}
	if up.ErasureCode == nil {
		up.ErasureCode, _ = NewRSCode(defaultDataPieces, defaultParityPieces)
	}
	if up.PieceSize == 0 {
		if fileInfo.Size() > defaultPieceSize {
			up.PieceSize = defaultPieceSize
		} else {
			up.PieceSize = smallPieceSize
		}
	}

	// Check that we have enough money to finance the upload.
	err = r.checkWalletBalance(up)
	if err != nil {
		return err
	}

	// Create file object.
	f := newFile(up.Nickname, up.ErasureCode, up.PieceSize, uint64(fileInfo.Size()))
	f.mode = uint32(fileInfo.Mode())

	// Select and connect to hosts.
	totalsize := up.PieceSize * uint64(up.ErasureCode.NumPieces()) * f.numChunks()
	var hosts []uploader
	randHosts := r.hostDB.RandomHosts(up.ErasureCode.NumPieces())
	for i := range randHosts {
		hostUploader, err := r.newHostUploader(randHosts[i], totalsize, up.Duration, f.masterKey)
		if err != nil {
			r.log.Printf("Upload: could not form contract with %v: %v", randHosts[i].IPAddress, err)
			continue
		}
		defer hostUploader.Close()
		hosts = append(hosts, hostUploader)
	}
	if len(hosts) < up.ErasureCode.MinPieces() {
		return errors.New("not enough hosts to support upload")
	}

	// Add file to renter.
	lockID = r.mu.Lock()
	r.files[up.Nickname] = f
	r.save()
	r.mu.Unlock(lockID)

	// Upload in parallel.
	err = f.upload(handle, hosts)
	if err != nil {
		// Upload failed; remove the file object.
		lockID = r.mu.Lock()
		delete(r.files, up.Nickname)
		r.save()
		r.mu.Unlock(lockID)
		return errors.New("failed to upload any file pieces")
	}

	// Add file to repair set.
	lockID = r.mu.Lock()
	r.repairSet[up.Nickname] = up.Filename
	r.save()
	r.mu.Unlock(lockID)

	// Save the .sia file to the renter directory.
	err = r.saveFile(f)
	if err != nil {
		return err
	}

	return nil
}
