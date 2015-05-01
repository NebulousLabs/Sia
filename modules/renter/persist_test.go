package renter

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules/tester"
)

// TestRenterSaveAndLoad probes the save and load methods of the renter type.
func TestRenterSaveAndLoad(t *testing.T) {
	rt := newRenterTester("TestRenterSaveAndLoad", t)

	rt.renter.files["1"] = &file{
		Name:     "1",
		Checksum: crypto.HashObject("fake id"),

		Pieces: []filePiece{
			filePiece{
				Active:    true,
				Repairing: true,
			},
			filePiece{
				Active:    true,
				Repairing: false,
			},
		},

		renter: rt.renter,
	}
	rt.renter.files["2"] = &file{
		Name: "2",
		Pieces: []filePiece{
			filePiece{
				Active:    false,
				Repairing: true,
			},
			filePiece{
				Active:    false,
				Repairing: false,
			},
		},
	}
	lockID := rt.renter.mu.Lock()
	rt.renter.save()
	rt.renter.mu.Unlock(lockID)

	// Create a new renter that calls load.
	r, err := New(rt.cs, rt.hostdb, rt.wallet, rt.renter.saveDir)
	if err != nil {
		t.Fatal(err)
	}
	lockID = rt.renter.mu.Lock()
	err = r.load()
	rt.renter.mu.Unlock(lockID)
	if err != nil {
		t.Fatal(err)
	}

	if r.files["1"].Name != "1" {
		t.Error("Name didn't load correctly")
	}
	if r.files["1"].Checksum != crypto.HashObject("fake id") {
		t.Error("Checksum didn't load correctly")
	}
	if r.files["1"].Pieces[0].Active != true {
		t.Error("Pieces.Active didn't load correctly")
	}
	if r.files["1"].Pieces[0].Repairing != true {
		t.Error("Pieces.Repairing didn't load correctly")
	}
	if r.files["1"].Pieces[1].Active != true {
		t.Error("Pieces.Active didn't load correctly")
	}
	if r.files["1"].Pieces[1].Repairing != false {
		t.Error("Pieces.Repairing didn't load correctly")
	}
	if r.files["2"].Name != "2" {
		t.Error("Name didn't load correctly")
	}
	if r.files["2"].Pieces[0].Active != false {
		t.Error("Pieces.Active didn't load correctly")
	}
	if r.files["2"].Pieces[0].Repairing != true {
		t.Error("Pieces.Repairing didn't load correctly")
	}
	if r.files["2"].Pieces[1].Active != false {
		t.Error("Pieces.Active didn't load correctly")
	}
	if r.files["2"].Pieces[1].Repairing != false {
		t.Error("Pieces.Repairing didn't load correctly")
	}

	// Check that the mutex for the files was set correctly.
	_ = r.files["1"].Nickname() // will panic if mutex is wrong.

	// Read the file into the persist structure and try various forms of
	// corruption.
	persistFile := filepath.Join(r.saveDir, PersistFilename)
	persistBytes, err := ioutil.ReadFile(persistFile)
	if err != nil {
		t.Fatal(err)
	}
	var rp RenterPersistence
	err = json.Unmarshal(persistBytes, &rp)
	if err != nil {
		t.Fatal(err)
	}

	// Change the header.
	rp.Header = "bad"
	badBytes, err := json.Marshal(rp)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(persistFile, badBytes, 0660)
	if err != nil {
		t.Fatal(err)
	}
	lockID = rt.renter.mu.Lock()
	err = r.load()
	rt.renter.mu.Unlock(lockID)
	if err != ErrUnrecognizedHeader {
		t.Error("Expecting ErrUnrecognizedHeader:", err)
	}

	// Change the version.
	rp.Header = PersistHeader
	rp.Version = "bad"
	badBytes, err = json.Marshal(rp)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(persistFile, badBytes, 0660)
	if err != nil {
		t.Fatal(err)
	}
	lockID = rt.renter.mu.Lock()
	err = r.load()
	rt.renter.mu.Unlock(lockID)
	if err != ErrUnrecognizedVersion {
		t.Error("Expecting ErrUnrecognizedVersion:", err)
	}

	// Corrupt the file
	err = ioutil.WriteFile(persistFile, badBytes[1:], 0660)
	if err != nil {
		t.Fatal(err)
	}
	lockID = rt.renter.mu.Lock()
	err = r.load()
	rt.renter.mu.Unlock(lockID)
	if err == nil {
		t.Error("failed to corrupt the file")
	}
}

// TestFileSharing probes the LoadSharedFile and the ShareFile methods of the
// renter.
func TestFileSharing(t *testing.T) {
	// Create a directory to put all the files shared between the renters in
	// this test.
	shareDir := tester.TempDir("renter", "TestFileSharing")
	err := os.MkdirAll(shareDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	rt1 := newRenterTester("TestFileSharing - 1", t)
	rt2 := newRenterTester("TestFileSharing - 2", t)

	// Try to share a file from an empty renter.
	err = rt1.renter.ShareFiles([]string{"dne"}, filepath.Join(shareDir, "badshare.sia"))
	if err != ErrUnknownNickname {
		t.Error("Expecting ErrUnknownNickname:", err)
	}

	// Add a file to rt1 and share it with rt2.
	rt1.renter.files["1"] = &file{
		Name:     "1",
		Checksum: crypto.HashObject("fake id"),

		Pieces: []filePiece{
			filePiece{
				Active:    true,
				Repairing: true,
			},
			filePiece{
				Active:    true,
				Repairing: false,
			},
		},

		renter: rt1.renter,
	}
	err = rt1.renter.ShareFiles([]string{"1"}, filepath.Join(shareDir, "1share.sia"))
	if err != nil {
		t.Fatal(err)
	}
	err = rt2.renter.LoadSharedFile(filepath.Join(shareDir, "1share.sia"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rt2.renter.files) != 1 {
		t.Error("rt2 did not load the shared file")
	}

	// Try sharing nothing, and using an incorrect suffix.
	err = rt1.renter.ShareFiles([]string{}, filepath.Join(shareDir, "2share.sia"))
	if err != ErrNoNicknames {
		t.Error("Expecting ErrNoNicknames")
	}
	err = rt1.renter.ShareFiles([]string{"1"}, filepath.Join(shareDir, "3share.sia1"))
	if err != ErrNonShareSuffix {
		t.Error("Expecting ErrNonShareSuffix", err)
	}

	// Load a non-existant file.
	err = rt1.renter.LoadSharedFile(filepath.Join(shareDir, "0share.sia"))
	if err == nil {
		t.Error("expected error")
	}

	// Create and load a corrupt file.
	shareBytes, err := ioutil.ReadFile(filepath.Join(shareDir, "1share.sia"))
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(filepath.Join(shareDir, "1share.sia"), shareBytes[1:], 0660)
	if err != nil {
		t.Fatal(err)
	}
	err = rt1.renter.LoadSharedFile(filepath.Join(shareDir, "1share.sia"))
	if err == nil {
		t.Error("Expecting corruption error")
	}
}
