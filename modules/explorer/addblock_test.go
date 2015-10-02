package explorer

import (
	"errors"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// Generates a dummy hash filled with a given number
func genHashNum(x byte) crypto.Hash {
	var h crypto.Hash
	for i := 0; i < crypto.HashSize; i++ {
		h[i] = x
	}
	return h
}

// Add a couple blocks to the database, then perform lookups to see if
// they were added and crossed referenced correctly
func (et *explorerTester) testAddBlock(t *testing.T) error {
	if testing.Short() {
		t.SkipNow()
	}

	// This block will *NOT* be valid, but should contain
	// addresses that can cross reference each other.
	b1 := types.Block{
		ParentID:  types.BlockID(genHashNum(1)),
		Nonce:     [8]byte{2, 2, 2, 2, 2, 2, 2, 2},
		Timestamp: 3,
		MinerPayouts: []types.SiacoinOutput{types.SiacoinOutput{
			Value:      types.NewCurrency64(4),
			UnlockHash: types.UnlockHash(genHashNum(5)),
		}},
		Transactions: nil,
	}

	// This should not error at least...
	lockID := et.explorer.mu.Lock()
	err := et.explorer.addBlockDB(b1)
	et.explorer.mu.Unlock(lockID)
	if err != nil {
		return errors.New("Error inserting basic block: " + err.Error())
	}

	// Again, not a valid block at all.
	b2 := types.Block{
		ParentID:     b1.ID(),
		Nonce:        [8]byte{7, 7, 7, 7, 7, 7, 7, 7},
		Timestamp:    8,
		MinerPayouts: nil,
		Transactions: []types.Transaction{types.Transaction{
			SiacoinInputs: []types.SiacoinInput{types.SiacoinInput{
				ParentID: b1.MinerPayoutID(0),
			}},
			FileContracts: []types.FileContract{types.FileContract{
				UnlockHash: types.UnlockHash(genHashNum(10)),
			}},
		}},
	}

	lockID = et.explorer.mu.Lock()
	err = et.explorer.addBlockDB(b2)
	et.explorer.mu.Unlock(lockID)
	if err != nil {
		return errors.New("Error inserting block 2: " + err.Error())
	}

	// Now query the database to see if it has been linked properly
	lockID = et.explorer.mu.RLock()
	bytes, err := et.explorer.db.getFromBucket("Blocks", encoding.Marshal(b1.ID()))
	et.explorer.mu.RUnlock(lockID)
	var b types.Block
	err = encoding.Unmarshal(bytes, &b)
	if err != nil {
		return errors.New("Could not decode loaded block")
	}
	if b.ID() != b1.ID() {
		return errors.New("Block 1 not stored properly")
	}

	// Query to see if the input is added to the output field
	lockID = et.explorer.mu.RLock()
	bytes, err = et.explorer.db.getFromBucket("SiacoinOutputs", encoding.Marshal(b1.MinerPayoutID(0)))
	et.explorer.mu.RUnlock(lockID)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes == nil {
		return errors.New("Output is nil")
	}
	var ot outputTransactions
	err = encoding.Unmarshal(bytes, &ot)
	if err != nil {
		return errors.New("Could not decode loaded block")
	}
	if ot.InputTx == (types.TransactionID{}) {
		return errors.New("Input not added as output")
	}
	return nil
}

func TestAddBlock(t *testing.T) {
	et, err := createExplorerTester("TestExplorerAddBlock", t)
	if err != nil {
		t.Fatal(err)
	}
	err = et.testAddBlock(t)
	if err != nil {
		t.Fatal(err)
	}
}
