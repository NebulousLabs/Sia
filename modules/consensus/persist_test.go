package consensus

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
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
	cst.testBlockSuite()
	oldHash := cst.cs.dbConsensusChecksum()
	cst.cs.Close()

	// Reassigning this will lose subscribers and such, but we
	// just want to call load and get a hash
	g, err := gateway.New(":0", build.TempDir(modules.ConsensusDir, "TestSaveLoad", modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(build.SiaTestingDir, modules.ConsensusDir, "TestSaveLoad", modules.ConsensusDir)
	cst.cs, err = New(g, d)
	if err != nil {
		t.Fatal(err)
	}
	newHash := cst.cs.dbConsensusChecksum()
	if oldHash != newHash {
		t.Fatal("consensus set hash changed after load")
	}
}
