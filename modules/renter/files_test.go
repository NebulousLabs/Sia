package renter

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestFileAvailable probes the Available method of the file type.
func TestFileAvailable(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
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
	if testing.Short() {
		t.SkipNow()
	}
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

// TestRenterDeleteFile probes the DeleteFile method of the renter type.
func TestRenterDeleteFile(t *testing.T) {
	rt := newRenterTester("TestRenterDeleteFile", t)

	// Delete a file from an empty renter.
	err := rt.renter.DeleteFile("dne")
	if err != ErrUnknownNickname {
		t.Error("Expected ErrUnknownNickname:", err)
	}

	// Put a file in the renter.
	rt.renter.files["1"] = &file{
		Name:   "one",
		renter: rt.renter,
	}
	// Delete a different file.
	err = rt.renter.DeleteFile("one")
	if err != ErrUnknownNickname {
		t.Error("Expected ErrUnknownNickname:", err)
	}
	// Delete the file.
	err = rt.renter.DeleteFile("1")
	if err != nil {
		t.Error(err)
	}
	if len(rt.renter.FileList()) != 0 {
		t.Error("file was deleted, but is still reported in FileList?")
	}

	// Put a file in the renter, then rename it.
	rt.renter.files["1"] = &file{
		Name:   "one",
		renter: rt.renter,
	}
	rt.renter.RenameFile("1", "one")
	// Call delete on the previous name.
	err = rt.renter.DeleteFile("1")
	if err != ErrUnknownNickname {
		t.Error("Expected ErrUnknownNickname:", err)
	}
	// Call delete on the new name.
	err = rt.renter.DeleteFile("one")
	if err != nil {
		t.Error(err)
	}
}

// TestRenterFileList probes the FileList method of the renter type.
func TestRenterFileList(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt := newRenterTester("TestRenterFileList", t)

	// Get the file list of an empty renter.
	if len(rt.renter.FileList()) != 0 {
		t.Error("FileList has non-zero length for empty renter?")
	}

	// Put a file in the renter.
	rt.renter.files["1"] = &file{
		Name:   "one",
		renter: rt.renter,
	}
	if len(rt.renter.FileList()) != 1 {
		t.Error("FileList is not returning the only file in the renter")
	}
	if rt.renter.FileList()[0].Nickname() != "one" {
		t.Error("FileList is not returning the correct filename for the only file")
	}

	// Put multiple files in the renter.
	rt.renter.files["2"] = &file{
		Name:   "two",
		renter: rt.renter,
	}
	if len(rt.renter.FileList()) != 2 {
		t.Error("FileList is not returning both files in the renter")
	}
	files := rt.renter.FileList()
	if !((files[0].Nickname() == "one" || files[0].Nickname() == "two") &&
		(files[1].Nickname() == "one" || files[1].Nickname() == "two") &&
		(files[0].Nickname() != files[1].Nickname())) {
		t.Error("FileList is returning wrong names for the files:", files[0].Nickname(), files[1].Nickname())
	}
}

// TestRenterRenameFile probes the rename method of the renter.
func TestRenterRenameFile(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt := newRenterTester("TestRenterRenameFile", t)

	// Rename a file that doesn't exist.
	err := rt.renter.RenameFile("1", "1a")
	if err != ErrUnknownNickname {
		t.Error("Expecting ErrUnknownNickname:", err)
	}

	// Rename a file that does exist.
	rt.renter.files["1"] = &file{
		Name:   "1",
		renter: rt.renter,
	}
	files := rt.renter.FileList()
	err = rt.renter.RenameFile("1", "1a")
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.renter.FileList()) != 1 {
		t.Fatal("FileList has unexpected number of files:", len(rt.renter.FileList()))
	}
	if files[0].Nickname() != "1a" {
		t.Error("RenameFile failed, new file nickname is not what is expected.")
	}

	// Rename a file to an existing name.
	rt.renter.files["1"] = &file{
		Name:   "1",
		renter: rt.renter,
	}
	err = rt.renter.RenameFile("1", "1a")
	if err != ErrNicknameOverload {
		t.Error("Expecting ErrNicknameOverload:", err)
	}
	if files[0].Nickname() != "1a" {
		t.Error("Side effect occured during rename:", files[0].Nickname())
	}

	// Rename a file to the same name.
	err = rt.renter.RenameFile("1", "1")
	if err != ErrNicknameOverload {
		t.Error("Expecting ErrNicknameOverload:", err)
	}
	if files[0].Nickname() != "1a" {
		t.Error("Side effect occured during rename:", files[0].Nickname())
	}
}
