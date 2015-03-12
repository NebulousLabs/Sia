package renter

import (
	"errors"
	"io"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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

	// We only need one piece, so iterate through the hosts until a download
	// succeeds.
	for _, piece := range pieces {
		downloadErr := r.downloadPiece(piece, filename)
		if downloadErr == nil {
			return nil
		} else {
			// log error
		}
		// r.hostDB.FlagHost(piece.Host.IPAddress)
	}

	return errors.New("Too many hosts returned errors - could not recover the file")
}
