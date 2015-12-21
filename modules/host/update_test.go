package host

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

func TestStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestStorageProof")
	if err != nil {
		t.Fatal(err)
	}

	// create a file contract
	fc := types.FileContract{
		WindowStart:        types.MaturityDelay + 3,
		WindowEnd:          1000,
		Payout:             types.NewCurrency64(1),
		UnlockHash:         types.UnlockConditions{}.UnlockHash(),
		ValidProofOutputs:  []types.SiacoinOutput{{Value: types.NewCurrency64(1)}},
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(1)}},
	}
	txnBuilder := ht.wallet.StartTransaction()
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
	data := make([]byte, dataSize)
	rand.Read(data)
	root, err := crypto.ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	ioutil.WriteFile(filepath.Join(ht.host.persistDir, "foo"), data, 0777)

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
	revTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
	}

	// create obligation
	obligation := &contractObligation{
		ID:           fcid,
		FileContract: fc,
		Path:         filepath.Join(ht.host.persistDir, "foo"),
	}
	ht.host.obligationsByID[fcid] = obligation
	ht.host.obligationsByHeight[fc.WindowStart+1] = []*contractObligation{obligation}

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
}
