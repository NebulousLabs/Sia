package renter

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/encoding"
)

// savedFiles contains the list of all the files that have been saved by the
// renter.
type savedFiles struct {
	FilePieces []FilePiece
	Nickname   string
}

// save puts all of the files known to the renter on disk.
func (r *Renter) save() error {
	// create slice of savedFiles
	savedPieces := make([]savedFiles, 0, len(r.files))
	for nickname, file := range r.files {
		savedPieces = append(savedPieces, savedFiles{file.pieces, nickname})
	}
	return encoding.WriteFile(filepath.Join(r.saveDir, "files.dat"), savedPieces)
}

// load loads all of the files from disk.
func (r *Renter) load() error {
	var pieces []savedFiles
	err := encoding.ReadFile(filepath.Join(r.saveDir, "files.dat"), &pieces)
	if err != nil {
		return err
	}
	for _, piece := range pieces {
		r.files[piece.Nickname] = File{
			nickname: piece.Nickname,
			pieces:   piece.FilePieces,
			renter:   r,
		}
	}
	return nil
}
