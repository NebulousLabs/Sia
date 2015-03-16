package renter

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	downloadAttempts = 5
)

// A Download is a file download that has been queued by the renter.
type Download struct {
	// Implementation note: received is declared first to ensure that it is
	// 64-bit aligned. This is necessary to ensure that atomic operations work
	// correctly on ARM and x86-32.
	received uint64

	complete    bool
	filesize    uint64
	destination string
	nickname    string

	pieces  []FilePiece
	file    *os.File
	gateway modules.Gateway
}

// Complete returns whether the file is ready to be used.
func (d *Download) Complete() bool {
	return d.complete
}

// Filesize returns the size of the file.
func (d *Download) Filesize() uint64 {
	return d.filesize
}

// Received returns the number of bytes downloaded so far.
func (d *Download) Received() uint64 {
	return d.received
}

// Destination returns the file's location on disk.
func (d *Download) Destination() string {
	return d.destination
}

// Nickname returns the identifier assigned to the file when it was uploaded.
func (d *Download) Nickname() string {
	return d.nickname
}

// Write implements the io.Writer interface. Each write updates the Download's
// received field. This allows download progress to be monitored in real-time.
func (d *Download) Write(b []byte) (int, error) {
	n, err := d.file.Write(b)
	// atomically update d.received
	atomic.AddUint64(&d.received, uint64(n))
	return n, err
}

// downloadPiece attempts to retrieve a file piece from a host.
func (d *Download) downloadPiece(piece FilePiece) error {
	return d.gateway.RPC(piece.HostIP, "RetrieveFile", func(conn modules.NetConn) error {
		// Send the ID of the contract for the file piece we're requesting.
		if err := conn.WriteObject(piece.ContractID); err != nil {
			return err
		}

		// Simultaneously download the file and calculate its Merkle root.
		tee := io.TeeReader(
			// Use a LimitedReader to ensure we don't read indefinitely.
			io.LimitReader(conn, int64(piece.Contract.FileSize)),
			// Each byte we read from tee will also be written to file.
			d,
		)
		merkleRoot, err := crypto.ReaderMerkleRoot(tee)
		if err != nil {
			return err
		}

		if merkleRoot != piece.Contract.FileMerkleRoot {
			return errors.New("host provided a file that's invalid")
		}

		return nil
	})
}

// start initiates the download of a File.
func (d *Download) start() {
	// We only need one piece, so iterate through the hosts until a download
	// succeeds.
	for i := 0; i < downloadAttempts; i++ {
		for _, piece := range d.pieces {
			downloadErr := d.downloadPiece(piece)
			if downloadErr == nil {
				d.complete = true
				d.file.Close()
				return
			}
			// Reset seek, since the file may have been partially written. The
			// next attempt will overwrite these bytes.
			d.file.Seek(0, 0)
		}

		// This iteration failed, no hosts returned the piece. Try again
		// after waiting a random amount of time.
		randSource := make([]byte, 1)
		rand.Read(randSource)
		time.Sleep(time.Second * time.Duration(i*i) * time.Duration(randSource[0]))
	}

	// File could not be downloaded; delete the copy on disk.
	d.file.Close()
	os.Remove(d.destination)

	// TODO: log?
}

// newDownload initializes a new Download object.
func newDownload(file File, destination string) (*Download, error) {
	// Create the download destination file.
	handle, err := os.Create(destination)
	if err != nil {
		return nil, err
	}

	// Filter out the inactive pieces.
	var activePieces []FilePiece
	for _, piece := range file.pieces {
		if piece.Active {
			activePieces = append(activePieces, piece)
		}
	}
	if len(activePieces) == 0 {
		return nil, errors.New("no active pieces")
	}

	return &Download{
		complete: false,
		// for now, all the pieces are equivalent
		filesize:    file.pieces[0].Contract.FileSize,
		received:    0,
		destination: destination,
		nickname:    file.nickname,

		pieces:  activePieces,
		file:    handle,
		gateway: file.renter.gateway,
	}, nil
}

// Download downloads a file, identified by its nickname, to the destination
// specified.
func (r *Renter) Download(nickname, destination string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Lookup the File associated with the nickname.
	file, exists := r.files[nickname]
	if !exists {
		return errors.New("no file of that nickname")
	}

	// Create the download object and spawn the download process.
	d, err := newDownload(file, destination)
	if err != nil {
		return err
	}
	go d.start()

	// Add the download to the download queue.
	r.downloadQueue = append(r.downloadQueue, d)
	return nil
}

// DownloadQueue returns the list of downloads in the queue.
func (r *Renter) DownloadQueue() []modules.DownloadInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	downloads := make([]modules.DownloadInfo, len(r.downloadQueue))
	for i := range r.downloadQueue {
		downloads[i] = r.downloadQueue[i]
	}
	return downloads
}
