package renter

import (
	"io/ioutil"

	"github.com/NebulousLabs/Sia/encoding"
)

type savedFilePieces struct {
	FilePieces []FilePiece
	Nickname   string
}

func (r *Renter) save(filename string) (err error) {
	// create slice of savedFilePieces
	savedPieces := make([]savedFilePieces, 0, len(r.files))
	for nickname, pieces := range r.files {
		savedPieces = append(savedPieces, savedFilePieces{pieces, nickname})
	}

	err = ioutil.WriteFile(filename, encoding.Marshal(savedPieces), 0666)
	if err != nil {
		return
	}

	return
}

func (r *Renter) load(filename string) (err error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	var pieces []savedFilePieces
	err = encoding.Unmarshal(contents, &pieces)
	if err != nil {
		return
	}
	for _, piece := range pieces {
		r.files[piece.Nickname] = piece.FilePieces
	}
	return
}
