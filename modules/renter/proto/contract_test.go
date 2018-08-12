package proto

import (
	"bytes"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestContractUncommittedTxn tests that if a contract revision is left in an
// uncommitted state, either version of the contract can be recovered.
func TestContractUncommittedTxn(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create contract set with one contract
	dir := build.TempDir(filepath.Join("proto", t.Name()))
	cs, err := NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	initialHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    1,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
	}
	initialRoots := []crypto.Hash{{1}}
	c, err := cs.managedInsertContract(initialHeader, initialRoots)
	if err != nil {
		t.Fatal(err)
	}

	// apply an update to the contract, but don't commit it
	sc := cs.mustAcquire(t, c.ID)
	revisedHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    2,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
		StorageSpending: types.NewCurrency64(7),
		UploadSpending:  types.NewCurrency64(17),
	}
	revisedRoots := []crypto.Hash{{1}, {2}}
	fcr := revisedHeader.Transaction.FileContractRevisions[0]
	newRoot := revisedRoots[1]
	storageCost := revisedHeader.StorageSpending.Sub(initialHeader.StorageSpending)
	bandwidthCost := revisedHeader.UploadSpending.Sub(initialHeader.UploadSpending)
	walTxn, err := sc.recordUploadIntent(fcr, newRoot, storageCost, bandwidthCost)
	if err != nil {
		t.Fatal(err)
	}

	// the state of the contract should match the initial state
	// NOTE: can't use reflect.DeepEqual for the header because it contains
	// types.Currency fields
	merkleRoots, err := sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get merkle roots", err)
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(initialHeader)) {
		t.Fatal("contractHeader should match initial contractHeader")
	} else if !reflect.DeepEqual(merkleRoots, initialRoots) {
		t.Fatal("Merkle roots should match initial Merkle roots")
	}

	// close and reopen the contract set
	cs.Close()
	cs, err = NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	// the uncommitted transaction should be stored in the contract
	sc = cs.mustAcquire(t, c.ID)
	if len(sc.unappliedTxns) != 1 {
		t.Fatal("expected 1 unappliedTxn, got", len(sc.unappliedTxns))
	} else if !bytes.Equal(sc.unappliedTxns[0].Updates[0].Instructions, walTxn.Updates[0].Instructions) {
		t.Fatal("WAL transaction changed")
	}
	// the state of the contract should match the initial state
	merkleRoots, err = sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get merkle roots:", err)
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(initialHeader)) {
		t.Fatal("contractHeader should match initial contractHeader", sc.header, initialHeader)
	} else if !reflect.DeepEqual(merkleRoots, initialRoots) {
		t.Fatal("Merkle roots should match initial Merkle roots")
	}

	// apply the uncommitted transaction
	err = sc.commitTxns()
	if err != nil {
		t.Fatal(err)
	}
	// the uncommitted transaction should be gone now
	if len(sc.unappliedTxns) != 0 {
		t.Fatal("expected 0 unappliedTxns, got", len(sc.unappliedTxns))
	}
	// the state of the contract should now match the revised state
	merkleRoots, err = sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get merkle roots:", err)
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(revisedHeader)) {
		t.Fatal("contractHeader should match revised contractHeader", sc.header, revisedHeader)
	} else if !reflect.DeepEqual(merkleRoots, revisedRoots) {
		t.Fatal("Merkle roots should match revised Merkle roots")
	}
}
