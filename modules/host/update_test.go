package host

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// TestStorageProof checks that the host can create and submit a storage proof.
func TestStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestStorageProof")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// create a file contract
	fc := types.FileContract{
		WindowStart:        types.MaturityDelay + 3,
		WindowEnd:          1000,
		Payout:             types.NewCurrency64(1),
		UnlockHash:         types.UnlockConditions{}.UnlockHash(),
		ValidProofOutputs:  []types.SiacoinOutput{{Value: types.NewCurrency64(1)}, {Value: types.NewCurrency64(0)}},
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(1)}, {Value: types.NewCurrency64(0)}},
	}
	txnBuilder, err := ht.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	err = txnBuilder.FundSiacoins(fc.Payout)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddFileContract(fc)
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	fcid := signedTxnSet[len(signedTxnSet)-1].FileContractID(0)

	// generate data
	const dataSize = 777
	data := fastrand.Bytes(dataSize)
	root := crypto.MerkleRoot(data)
	err = ioutil.WriteFile(filepath.Join(ht.host.persistDir, "foo"), data, 0777)
	if err != nil {
		t.Fatal(err)
	}

	// create revision
	rev := types.FileContractRevision{
		ParentID:              fcid,
		UnlockConditions:      types.UnlockConditions{},
		NewFileSize:           dataSize,
		NewWindowStart:        fc.WindowStart,
		NewFileMerkleRoot:     root,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
		NewRevisionNumber:     1,
	}
	_ = types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
	}

	/*
		// create obligation
		obligation := &contractObligation{
			ID: fcid,
			OriginTransaction: types.Transaction{
				FileContracts: []types.FileContract{fc},
			},
			Path: filepath.Join(ht.host.persistDir, "foo"),
		}
		ht.host.obligationsByID[fcid] = obligation
		ht.host.addActionItem(fc.WindowStart+1, obligation)

		// submit both to tpool
		err = ht.tpool.AcceptTransactionSet(append(signedTxnSet, revTxn))
		if err != nil {
			t.Fatal(err)
		}
		_, err = ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}

		// storage proof will be submitted after mining one more block
		_, err = ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	*/
}

// TestInitRescan probes the initRescan function, verifying that it works in
// the naive case. The rescan is triggered manually.
func TestInitRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestInitRescan")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Check that the host's persistent variables have incorporated the first
	// few blocks.
	if ht.host.recentChange == (modules.ConsensusChangeID{}) || ht.host.blockHeight == 0 {
		t.Fatal("host variables do not indicate that the host is tracking the consensus set correctly")
	}
	oldChange := ht.host.recentChange
	oldHeight := ht.host.blockHeight

	// Corrupt the variables and perform a rescan to see if they reset
	// correctly.
	ht.host.recentChange[0]++
	ht.host.blockHeight += 100e3
	ht.cs.Unsubscribe(ht.host)
	err = ht.host.initRescan()
	if err != nil {
		t.Fatal(err)
	}
	if oldChange != ht.host.recentChange || oldHeight != ht.host.blockHeight {
		t.Error("consensus tracking variables were not reset correctly after rescan")
	}
}

// TestIntegrationAutoRescan checks that a rescan is triggered during New if
// the consensus set becomes desynchronized.
func TestIntegrationAutoRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestIntegrationAutoRescan")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Check that the host's persistent variables have incorporated the first
	// few blocks.
	if ht.host.recentChange == (modules.ConsensusChangeID{}) || ht.host.blockHeight == 0 {
		t.Fatal("host variables do not indicate that the host is tracking the consensus set correctly")
	}
	oldChange := ht.host.recentChange
	oldHeight := ht.host.blockHeight

	// Corrupt the variables, then close the host.
	ht.host.recentChange[0]++
	ht.host.blockHeight += 100e3
	err = ht.host.Close() // host saves upon closing
	if err != nil {
		t.Fatal(err)
	}

	// Create a new host and check that the persist variables have correctly
	// reset.
	h, err := New(ht.cs, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	if oldChange != h.recentChange || oldHeight != h.blockHeight {
		t.Error("consensus tracking variables were not reset correctly after rescan")
	}

	// Set ht.host to 'h' so that the 'ht.Close()' method will close everything
	// cleanly.
	ht.host = h
}
