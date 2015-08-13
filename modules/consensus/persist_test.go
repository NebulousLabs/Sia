package consensus

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// TestSaveLoad populates a blockchain, saves it, loads it, and checks
// the consensus set hash before and after
func TestSaveLoad(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestSaveLoad")
	if err != nil {
		t.Fatal(err)
	}

	err = cst.complexBlockSet()
	if err != nil {
		t.Fatal(err)
	}

	oldHash := cst.cs.consensusSetHash()
	cst.cs.Close()

	// Reassigning this will loose subscribers and such, but we
	// just want to call load and get a hash
	d := filepath.Join(build.SiaTestingDir, filepath.Join(modules.ConsensusDir, filepath.Join("TestSaveLoad", modules.ConsensusDir)))
	cst.cs, err = New(cst.gateway, d)
	if err != nil {
		t.Fatal(err)
	}
	newHash := cst.cs.consensusSetHash()
	if oldHash != newHash {
		t.Fatal("consensus set hash changed after load")
	}
}

// TestConsistencyGuard verifies that the database cannot be modified after it
// has been corrupted
func TestConsistencyGuard(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestConsistencyGuard")
	if err != nil {
		t.Fatal(err)
	}

	// Improperly trigger the guard, simulating a situation where the guard is
	// added at the beginning of editing but not removed at the end of editing.
	err = cst.cs.db.startConsistencyGuard()
	if err != nil {
		t.Fatal(err)
	}

	_, err = cst.miner.AddBlock()
	if err != errDBInconsistent {
		t.Fatal(err)
	}
}
