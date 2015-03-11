package renter

import (
	"io/ioutil"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
)

// savedFiles contains the list of all the files that have been saved by the
// renter.
type savedFiles struct {
	FilePieces  []FilePiece
	Nickname    string
	StartHeight consensus.BlockHeight
}

// save puts all of the files known to the renter on disk.
func (r *Renter) save() (err error) {
	// create slice of savedFiles
	savedPieces := make([]savedFiles, 0, len(r.files))
	for nickname, file := range r.files {
		savedPieces = append(savedPieces, savedFiles{file.pieces, nickname, file.startHeight})
	}

	err = ioutil.WriteFile(r.persistentFile, encoding.Marshal(savedPieces), 0666)
	if err != nil {
		return
	}

	return
}

// load loads all of the files from disk.
func (r *Renter) load(filename string) (err error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	var pieces []savedFiles
	err = encoding.Unmarshal(contents, &pieces)
	if err != nil {
		return
	}
	for _, piece := range pieces {
		r.files[piece.Nickname] = File{
			nickname:    piece.Nickname,
			pieces:      piece.FilePieces,
			startHeight: piece.StartHeight,
			renter:      r,
		}
	}
	return
}
