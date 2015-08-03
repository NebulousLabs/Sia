package consensus

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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

func acceptRecover(cs *ConsensusSet, block types.Block) (err error) {
	defer func() {
		r := recover()
		switch x := r.(type) {
		case string:
			err = errors.New(x)
		case error:
			err = x
		default:
			err = errors.New("unknown panic")
		}
	}()
	return cs.AcceptBlock(block)
}

// TestConsistencyGuard verifies that the database cannot be modified
// after it has been corrupted
func TestConsistencyGuard(t *testing.T) {
	cst, err := createConsensusSetTester("TestConsistencyGuard")
	if err != nil {
		t.Fatal(err)
	}

	blockA, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(blockA)

	blockB, _ := cst.miner.FindBlock()
	// Change the database so that accepting the next block will
	// trigger a panic
	pbA := cst.cs.db.getBlockMap(blockA.ID())
	pbA.Parent = types.ZeroID
	cst.cs.db.updateBlockMap(pbA)
	err = acceptRecover(cst.cs, blockB)
	if err != errNilItem {
		t.Fatal(err)
	}

	// Attempting to accept this block should now throw an
	// inconsistent Database error
	err = cst.cs.AcceptBlock(blockB)
	if err != ErrInconsistentSet {
		t.Fatal(err)
	}
}
