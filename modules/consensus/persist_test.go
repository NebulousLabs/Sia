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

	oldHash := cst.cs.dbConsensusChecksum()
	cst.cs.Close()

	// Reassigning this will lose subscribers and such, but we
	// just want to call load and get a hash
	d := filepath.Join(build.SiaTestingDir, filepath.Join(modules.ConsensusDir, filepath.Join("TestSaveLoad", modules.ConsensusDir)))
	cst.cs, err = New(cst.gateway, d)
	if err != nil {
		t.Fatal(err)
	}
	newHash := cst.cs.dbConsensusChecksum()
	if oldHash != newHash {
		t.Fatal("consensus set hash changed after load")
	}
}
