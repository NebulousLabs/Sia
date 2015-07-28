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
