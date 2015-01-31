package renter

import (
	"errors"
	"io"
	"net"
	"os"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	maxUploadAttempts = 10
)

// uploadPiece attempts to negotiate a contract with a host. If the negotiate
// is successful, uploadPiece will upload the file contents to the host, and
// return a FilePiece object specifying the host chosen and the details of the
// contract.
func (r *Renter) uploadPiece(up modules.UploadParams) (piece FilePiece, err error) {
	// Try 'maxUploadAttempts' hosts before giving up.
	for attempts := 0; attempts < maxUploadAttempts; attempts++ {
		// Select a host. An error here is unrecoverable.
		host, hostErr := r.hostDB.RandomHost()
		if hostErr != nil {
			err = errors.New("could not get a host from the HostDB: " + hostErr.Error())
			return
		}

		// Create file contract using this host's parameters. An error here is
		// unrecoverable.
		t, fileContract, txnErr := r.createContractTransaction(host, up)
		if txnErr != nil {
			err = errors.New("unable to create contract transaction: " + txnErr.Error())
			return
		}

		// Negotiate the contract with the host. If the negotiation is
		// successful, the file will be uploaded.
		err = negotiateContract(host, t, up)
		if err != nil {
			// If there was a problem, we need to try again with a new host.
			r.hostDB.FlagHost(host.IPAddress)
			continue
		}

		// Otherwise, we're done.
		piece = FilePiece{
			Host:     host,
			Contract: fileContract,
		}
		return
	}

	err = errors.New("no hosts accepted the file contract")
	return
}

// Upload implements the modules.Renter interface. It selects a host to upload
// to, negotiates a contract with it, and uploads the file contents.
func (r *Renter) Upload(up modules.UploadParams) (err error) {
	r.mu.RLock()
	_, exists := r.files[up.Nickname]
	r.mu.RUnlock()
	if exists {
		return errors.New("file with that nickname already exists")
	}

	pieces := make([]FilePiece, up.Pieces)
	for i := range pieces {
		// upload the piece to a host. The host is chosen by uploadPiece.
		// TODO: what happens if we can't upload to all the hosts?
		pieces[i], err = r.uploadPiece(up)
		if err != nil {
			return
		}
	}

	r.mu.Lock()
	r.files[up.Nickname] = pieces
	r.mu.Unlock()
	return
}

// downloadPiece attempts to retrieve a file from a host.
func (r *Renter) downloadPiece(piece FilePiece, path string) error {
	return piece.Host.IPAddress.Call("RetrieveFile", func(conn net.Conn) (err error) {
		// send filehash
		if _, err = encoding.WriteObject(conn, piece.ContractID); err != nil {
			return
		}
		// TODO: read error

		// create file
		file, err := os.Create(path)
		if err != nil {
			return
		}
		defer file.Close()

		// copy response into file
		_, err = io.CopyN(file, conn, int64(piece.Contract.FileSize))
		if err != nil {
			os.Remove(path)
			return
		}
		return
	})
}

// Download implements the modules.Renter interface. It requests a file from
// the hosts it was stored with, and downloads it into the specified filename.
func (r *Renter) Download(nickname, filename string) error {
	r.mu.RLock()
	pieces, exists := r.files[nickname]
	r.mu.RUnlock()
	if !exists {
		return errors.New("no record of file: " + nickname)
	}

	// We only need one piece, so iterate through the hosts until a download
	// succeeds.
	// TODO: smarter ordering here? i.e. prioritize known fast hosts?
	for _, piece := range pieces {
		downloadErr := r.downloadPiece(piece, filename)
		if downloadErr == nil {
			return nil
		}
		// log?
		r.hostDB.FlagHost(piece.Host.IPAddress)
	}

	return errors.New("Too many hosts returned errors - could not recover the file")
}
