package renter

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestFileAvailable probes the Available method of the file type.
func TestFileAvailable(t *testing.T) {
	rt := newRenterTester("TestFileAvailable", t)
	f := file{
		PiecesRequired: 1,
		Pieces: []filePiece{
			filePiece{Active: false},
			filePiece{Active: false},
			filePiece{Active: false},
		},

		renter: rt.renter,
	}

	// Try a file with no active pieces.
	if f.Available() {
		t.Error("f is not supposed to be available with 1 required and 0 active.")
	}
	// Try with one active and one required peer.
	f.Pieces[0].Active = true
	if !f.Available() {
		t.Error("f is supposed to be available with 1 required and 1 active")
	}
	// Try with multiple required pieces, but 1 active peer.
	f.PiecesRequired = 2
	if f.Available() {
		t.Error("f is not supposed to be available with 2 required and 1 active.")
	}
	// Try with multiple required pieces, enough of which are active.
	f.Pieces[2].Active = true
	if !f.Available() {
		t.Error("f is supposed to be available with 2 active and 2 required.")
	}
	// Try with more than enough active pieces.
	f.Pieces[1].Active = true
	if !f.Available() {
		t.Error("f is supposed to be available with 3 active and 2 required.")
	}
}

// TestFileNickname probes the Nickname method of the file type.
func TestFileNickname(t *testing.T) {
	rt := newRenterTester("TestFileNickname", t)

	// Try a file with no active pieces.
	f := file{
		Name:   "name",
		renter: rt.renter,
	}
	if f.Nickname() != "name" {
		t.Error("got the wrong nickname for a file")
	}
}

// TestFileRepairing probes the Repairing method of the file type.
func TestFileRepairing(t *testing.T) {
	rt := newRenterTester("TestFileRepairing", t)
	f := file{
		PiecesRequired: 1,
		Pieces: []filePiece{
			filePiece{Repairing: false},
			filePiece{Repairing: false},
		},

		renter: rt.renter,
	}

	// Try a file with no repairing pieces.
	if f.Repairing() {
		t.Error("file should not register as repairing")
	}
	// Try a file with one repairing piece.
	f.Pieces[1].Repairing = true
	if !f.Repairing() {
		t.Error("file should register as repairing")
	}
	// Try a file with all pieces repairing.
	f.Pieces[0].Repairing = true
	if !f.Repairing() {
		t.Error("file should register as repairing")
	}
}

// TestFileTimeRemaining probes the TimeRemaining method of the file type.
func TestFileTimeRemaining(t *testing.T) {
	rt := newRenterTester("TestFileTimeRemaining", t)
	f := file{
		renter: rt.renter,
	}

	// Try when there are no pieces.
	if f.TimeRemaining() != 0 {
		t.Error("file with no pieces should report as having no time remaining")
	}
	// Try when a piece has a contract which is expiring. 0 is acceptable as a
	// start time because newRenterTester has already mined a few blocks.
	f.Pieces = append(f.Pieces, filePiece{
		Contract: types.FileContract{
			WindowStart: 0,
		},
	})
	if f.TimeRemaining() != 0 {
		t.Error("file with expiring contract should report as having no time remaining")
	}
	f.Pieces[0].Contract.WindowStart = 100 + rt.renter.blockHeight
	if f.TimeRemaining() != 100 {
		t.Error("file should claim to be expiring in 100 blocks")
	}
}
