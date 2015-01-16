package renter

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/sia/components"
)

type FilePiece struct {
	Host     components.HostEntry   // Where to find the file.
	Contract consensus.FileContract // The contract being enforced.
}

type FileEntry struct {
	Pieces []FilePiece
}

type Renter struct {
	state  *consensus.State
	files  map[string]FileEntry
	hostDB components.HostDB
	wallet components.Wallet

	mu sync.RWMutex
}

func (r *Renter) RentInfo() (ri components.RentInfo, err error) {
	for key := range r.files {
		ri.Files = append(ri.Files, key)
	}
	return
}

func New(state *consensus.State, hdb components.HostDB, wallet components.Wallet) (r *Renter, err error) {
	if state == nil {
		err = errors.New("renter.New: cannot have nil state")
		return
	}
	if hdb == nil {
		err = errors.New("renter.New: cannot have nil hostDB")
		return
	}
	if wallet == nil {
		err = errors.New("renter.New: cannot have nil wallet")
		return
	}

	r = &Renter{
		state:  state,
		hostDB: hdb,
		wallet: wallet,
		files:  make(map[string]FileEntry),
	}
	return
}

func (r *Renter) RenameFile(currentName, newName string) error {
	// Check that the currentName exists and the newName doesn't.
	entry, exists := r.files[currentName]
	if !exists {
		return errors.New("no file found by that name")
	}
	_, exists = r.files[newName]
	if exists {
		return errors.New("file of new name already exists")
	}

	// Do the renaming.
	delete(r.files, currentName)
	r.files[newName] = entry
	return nil
}

func (r *Renter) downloadPiece(piece FilePiece, destination string) (err error) {
	return piece.Host.IPAddress.Call("RetrieveFile", func(conn net.Conn) error {
		// send filehash
		if _, err := encoding.WriteObject(conn, piece.Contract.FileMerkleRoot); err != nil {
			return err
		}
		// TODO: read error
		// copy response into file
		file, err := os.Create(destination)
		if err != nil {
			return err
		}
		_, err = io.CopyN(file, conn, int64(piece.Contract.FileSize))
		file.Close()
		if err != nil {
			os.Remove(destination)
		}
		return err
	})
}

// Download requests a file from the host it was stored with, and downloads it
// into the specified filename.
func (r *Renter) Download(nickname, filename string) (err error) {
	entry, exists := r.files[nickname]
	if !exists {
		return errors.New("no file entry for file: " + nickname)
	}

	// We just need to get one piece, we'll keep contacting hosts until one
	// doesn't return an error.
	for _, piece := range entry.Pieces {
		err = r.downloadPiece(piece, filename)
		if err == nil {
			return
		} else {
			fmt.Println(err)
			r.hostDB.FlagHost(piece.Host.ID)
		}
	}

	if err != nil {
		err = errors.New("Too many hosts returned errors - could not recover the file.")
		return
	}

	return
}
