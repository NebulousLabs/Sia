package blockexplorer

import (
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
func (et *explorerTester) testAddBlock(t *testing.T) {
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
	err := et.explorer.addBlockDB(b1)
	if err != nil {
		et.t.Fatal("Error inserting basic block: " + err.Error())
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

	err = et.explorer.addBlockDB(b2)
	if err != nil {
		et.t.Fatal("Error inserting block 2: " + err.Error())
	}

	// Now query the database to see if it has been linked properly
	bytes, err := et.explorer.db.GetFromBucket("Blocks", encoding.Marshal(b1.ID()))
	var b types.Block
	err = encoding.Unmarshal(bytes, &b)
	if err != nil {
		et.t.Fatal("Could not decode loaded block")
	}
	if b.ID() != b1.ID() {
		et.t.Fatal("Block 1 not stored properly")
	}

	// Query to see if the input is added to the output field
	bytes, err = et.explorer.db.GetFromBucket("Outputs", encoding.Marshal(b1))
	var ot outputTransactions
	err = encoding.Unmarshal(bytes, &ot)
	if err != nil {
		et.t.Fatal("Could not decode loaded block")
	}
	if ot.InputTx == *new(crypto.Hash) {
		et.t.Fatal("Input not added as output")
	}
}

func TestAddBlock(t *testing.T) {
	et := createExplorerTester("TestExplorerAddBlock", t)
	et.testConsensusUpdates(t)
}
