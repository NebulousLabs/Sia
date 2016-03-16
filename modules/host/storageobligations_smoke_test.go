package host

// storageobligations_smoke_test.go performs smoke testing on the the storage
// obligation management. This includes adding valid storage obligations, and
// waiting until they expire, to see if the failure modes are all handled
// correctly.

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// newTesterStorageObligation uses the wallet to create and fund a file
// contract that will form the foundation of a storage obligation.
func (ht *hostTester) newTesterStorageObligation() (*storageObligation, error) {
	// Create the file contract that will be used in the obligation.
	builder := ht.wallet.StartTransaction()
	// Fund the file contract with a payout.
	payout := types.NewCurrency64(10e9)
	err := builder.FundSiacoins(payout)
	if err != nil {
		return nil, err
	}
	// Add the file contract that consumes the funds.
	emptyRoot := crypto.MerkleRoot(nil)
	_ = builder.AddFileContract(types.FileContract{
		FileSize:       0,
		FileMerkleRoot: emptyRoot,
		WindowStart:    ht.host.blockHeight + 8,
		WindowEnd:      ht.host.blockHeight + 8 + defaultWindowSize,
		Payout:         payout,
		ValidProofOutputs: []types.SiacoinOutput{
			{
				Value: types.PostTax(ht.host.blockHeight, payout),
			},
			{
				Value: types.ZeroCurrency,
			},
		}, // 1 for renter, 1 for host. Renter retains all.
		MissedProofOutputs: []types.SiacoinOutput{
			{
				Value: types.PostTax(ht.host.blockHeight, payout),
			},
			{
				Value: types.ZeroCurrency,
			},
		}, // 1 for renter, 1 for host. Renter retains all.
		UnlockHash:     (types.UnlockConditions{}).UnlockHash(),
		RevisionNumber: 0,
	})
	// Sign the transaction.
	tSet, err := builder.Sign(true)
	if err != nil {
		return nil, err
	}

	// Assemble and return the storage obligation.
	so := &storageObligation{
		OriginTransactionSet: tSet,

		// There are no tracking values, because no fees were added.
	}
	return so, nil
}

// TestStorageObligationSmoke runs the smoke tests on the storage obligations.
func TestStorageObligationSmoke(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestStorageObligationSmoke")
	if err != nil {
		t.Fatal(err)
	}

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.addStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Mine a block to confirm the transaction containing the storage
	// obligation.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}
}
