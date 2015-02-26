package renter

import (
	"errors"
	"io"
	"net"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	maxUploadAttempts = 5
)

// downloadPiece attempts to retrieve a file from a host.
func downloadPiece(piece FilePiece, path string) error {
	return piece.Host.IPAddress.Call("RetrieveFile", func(conn net.Conn) (err error) {
		// Send the id of the contract for the file piece we're requesting. The
		// response will be the file piece contents.
		if _, err = encoding.WriteObject(conn, piece.ContractID); err != nil {
			return
		}

		// Create the file on disk.
		file, err := os.Create(path)
		if err != nil {
			return
		}
		defer file.Close()

		// Write the host's response into the file.
		_, err = io.CopyN(file, conn, int64(piece.Contract.FileSize))
		if err != nil {
			os.Remove(path)
			// r.hostDB.FlagHost(piece.Host.IPAddress)
			return
		}

		// Do an integrity check to make sure that the piece we were given is
		// actually what we were looking for.
		_, err = file.Seek(0, 0)
		if err != nil {
			return
		}
		merkleRoot, err := crypto.ReaderMerkleRoot(file, piece.Contract.FileSize)
		if err != nil {
			return
		}
		if merkleRoot != piece.Contract.FileMerkleRoot {
			return errors.New("host provided a file that's invalid")
		}

		return
	})
}

// threadedUploadPiece will upload the piece of a file to a randomly chosen
// host. If the wallet has insufficient balance to support uploading,
// uploadPiece will give up. The file uploading can be continued using a repair
// tool. Upon completion, the memory containg the piece's information is
// updated.
func (r *Renter) threadedUploadPiece(up modules.UploadParams, piece *FilePiece) {
	// Try 'maxUploadAttempts' hosts before giving up.
	for attempts := 0; attempts < maxUploadAttempts; attempts++ {
		// Select a host. An error here is unrecoverable.
		host, err := r.hostDB.RandomHost()
		if err != nil {
			return
		}

		// Negotiate the contract with the host. If the negotiation is
		// unsuccessful, we need to try again with a new host. Otherwise, the
		// file will be uploaded and we'll be done.
		contract, contractID, err := r.negotiateContract(host, up)
		if err != nil {
			continue
		}

		r.mu.Lock()
		*piece = FilePiece{
			Host:       host,
			Contract:   contract,
			ContractID: contractID,
			Active:     true,
		}
		r.mu.Unlock()
		return
	}
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
	for _, piece := range r.files[nickname] {
		if piece.Active {
			pieces = append(pieces, piece)
		}
	}
	r.mu.RUnlock()

	// We only need one piece, so iterate through the hosts until a download
	// succeeds.
	//
	// TODO: Multiple opportunities for optimization here but we should wait
	// until we're actually erasure coding.
	for _, piece := range pieces {
		downloadErr := downloadPiece(piece, filename)
		if downloadErr == nil {
			return nil
		}
		// r.hostDB.FlagHost(piece.Host.IPAddress)
	}

	return errors.New("Too many hosts returned errors - could not recover the file")
}

// Upload takes an upload parameters, which contain a file to upload, and then
// creates a redundant copy of the file on the Sia network.
func (r *Renter) Upload(up modules.UploadParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for a nickname conflict.
	pieces, exists := r.files[up.Nickname]
	if exists {
		return errors.New("file with that nickname already exists")
	}

	// Check that the hostdb is sufficiently large to support an upload. Right
	// now that value is set to 3, but in the future the logic will be a bit
	// more complex; once there is erasure coding we'll want to hit the minimum
	// number of pieces plus some buffer before we decide that an upload is
	// okay.
	if r.hostDB.NumHosts() < 3 {
		return errors.New("not enough hosts on the network to upload a file :( - maybe you need to upgrade your software")
	}

	// TODO: Have some sort of rolling estimate for how much the upload is
	// going to cost. Right now we just run in blind, with maybe not enough to
	// upload more than a few pieces. Not having enough money won't cause us to
	// get stuck, but it may confuse the user since there was no warning. Also,
	// depending on how long it takes to negotiate contracts, the balance may
	// not drop immediately.

	// Upload a piece to every host on the network.
	r.files[up.Nickname] = make([]FilePiece, up.Pieces)
	for i := range pieces {
		// TODO: Eventually, each piece is likely to have different
		// requirements. Erasure coding, index, etc. There will likely need to
		// be a 'filePieceParameters' struct which is more complicated than the
		// 'uploadParameters' struct. For now, because we're using perfect
		// redundancy, it is sufficient to just tell the uploading thread what
		// index it's using.

		// threadedUploadPiece will change the memory that the piece points to,
		// which is useful because it means the file itself can be renamed but
		// will still point to the same underlying pieces.
		go r.threadedUploadPiece(up, &pieces[i])
	}

	return nil
}
