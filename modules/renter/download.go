package renter

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// A Download is a download that has been queued by the renter.
type Download struct {
	completed   bool
	destination string
	nickname    string

	renter *Renter
}

var (
	downloadAttempts = 5
)

// downloadPiece attempts to retrieve a file from a host.
func (r *Renter) downloadPiece(piece FilePiece, path string) error {
	return r.gateway.RPC(piece.HostIP, "RetrieveFile", func(conn modules.NetConn) (err error) {
		// Send the id of the contract for the file piece we're requesting. The
		// response will be the file piece contents.
		if err = conn.WriteObject(piece.ContractID); err != nil {
			return
		}

		// Create the file on disk.
		file, err := os.Create(path)
		if err != nil {
			return
		}
		defer file.Close()

		// Simultaneously download file and calculate its Merkle root.
		tee := io.TeeReader(
			// use a LimitedReader to ensure we don't read indefinitely
			io.LimitReader(conn, int64(piece.Contract.FileSize)),
			// each byte we read from tee will also be written to file
			file,
		)
		merkleRoot, err := crypto.ReaderMerkleRoot(tee)
		if err != nil {
			return
		}

		if merkleRoot != piece.Contract.FileMerkleRoot {
			return errors.New("host provided a file that's invalid")
		}

		return
	})
}

// Download downloads a file. Mutex conventions are broken to prevent doing
// network communication with io in place.
func (r *Renter) Download(nickname, filename string) error {
	// Grab the set of pieces we're downloading.
	r.mu.RLock()
	var pieces []FilePiece
	_, exists := r.files[nickname]
	if !exists {
		r.mu.RUnlock()
		return errors.New("no file of that nickname")
	}
	for _, piece := range r.files[nickname].pieces {
		if piece.Active {
			pieces = append(pieces, piece)
		}
	}
	r.mu.RUnlock()

	// Create an object for the download in the download queue.
	r.mu.Lock()
	downloadIndex := len(r.downloadQueue)
	r.downloadQueue = append(r.downloadQueue, Download{
		completed:   false,
		destination: filename,
		nickname:    nickname,

		renter: r,
	})
	r.mu.Unlock()

	// We only need one piece, so iterate through the hosts until a download
	// succeeds.
	go func() {
		for i := 0; i < downloadAttempts; i++ {
			for _, piece := range pieces {
				downloadErr := r.downloadPiece(piece, filename)
				if downloadErr == nil {
					// Mark the download as complete.
					r.mu.Lock()
					r.downloadQueue[downloadIndex].completed = true
					r.mu.Unlock()
					return
				}
			}

			// This iteration failed, no hosts returned the piece. Try again
			// after waiting a random amount of time.
			randSource := make([]byte, 1)
			rand.Read(randSource)
			time.Sleep(time.Second * time.Duration(i*i) * time.Duration(randSource[0]))
		}
	}()

	return nil
}

// DownloadQueue returns the list of downloads in the queue.
func (r *Renter) DownloadQueue() []modules.DownloadInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	downloads := make([]modules.DownloadInfo, len(r.downloadQueue))
	for i := range r.downloadQueue {
		downloads[i] = &r.downloadQueue[i]
	}
	return downloads
}

// Getter for the completed status of the download.
func (d *Download) Completed() bool {
	d.renter.mu.RLock()
	defer d.renter.mu.RUnlock()
	return d.completed
}

// Getter for the destination of the download.
func (d *Download) Destination() string {
	d.renter.mu.RLock()
	defer d.renter.mu.RUnlock()
	return d.destination
}

// Getter for the nickname of the download.
func (d *Download) Nickname() string {
	d.renter.mu.RLock()
	defer d.renter.mu.RUnlock()
	return d.nickname
}
