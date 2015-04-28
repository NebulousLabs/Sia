package renter

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// TestRenterSaveAndLoad probes the save and load methods of the renter type.
func TestRenterSaveAndLoad(t *testing.T) {
	rt := newRenterTester("TestRenterSaveAndLoad", t)

	encryptionKey, err := crypto.GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}

	rt.renter.files["1"] = &file{
		Name:          "1",
		EncryptionKey: encryptionKey,
		Checksum:      crypto.HashObject("fake id"),

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
	r, err := New(rt.cs, rt.gateway, rt.hostdb, rt.wallet, rt.renter.saveDir)
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
	if r.files["1"].EncryptionKey != encryptionKey {
		t.Error("EncryptionKey didn't load correctly")
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
