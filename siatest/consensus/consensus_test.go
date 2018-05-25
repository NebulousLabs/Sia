package consensus

import (
	"reflect"
	"testing"

	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
	"github.com/NebulousLabs/Sia/types"
)

// TestApiHeight checks if the consensus api endpoint works
func TestApiHeight(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testdir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a new server
	testNode, err := siatest.NewNode(node.AllModules(testdir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testNode.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Send GET request
	cg, err := testNode.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	height := cg.Height

	// Mine a block
	if err := testNode.MineBlock(); err != nil {
		t.Fatal(err)
	}

	// Request height again and check if it increased
	cg, err = testNode.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	if cg.Height != height+1 {
		t.Fatal("Height should have increased by 1 block")
	}
}

// TestConsensusBlocksIDGet tests the /consensus/blocks endpoint
func TestConsensusBlocksIDGet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a testgroup
	groupParams := siatest.GroupParams{
		Hosts:   1,
		Renters: 1,
		Miners:  1,
	}
	tg, err := siatest.NewGroupFromTemplate(groupParams)
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	testNode := tg.Miners()[0]

	// Send /consensus request
	endBlock, err := testNode.ConsensusGet()
	if err != nil {
		t.Fatal("Failed to call ConsensusGet():", err)
	}

	// Loop over blocks and compare
	var i types.BlockHeight
	var zeroID types.BlockID
	for i = 0; i <= endBlock.Height; i++ {
		cbhg, err := testNode.ConsensusBlocksHeightGet(i)
		if err != nil {
			t.Fatal("Failed to retrieve block by height:", err)
		}
		cbig, err := testNode.ConsensusBlocksIDGet(cbhg.ID)
		if err != nil {
			t.Fatal("Failed to retrieve block by ID:", err)
		}
		// Confirm blocks received by both endpoints are the same
		if !reflect.DeepEqual(cbhg, cbig) {
			t.Fatal("Blocks not equal")
		}
		// Confirm Fields were set properly
		// Ignore ParentID and MinerPayouts for genisis block
		if cbig.ParentID == zeroID && i != 0 {
			t.Fatal("ParentID wasn't set correctly")
		}
		if len(cbig.MinerPayouts) == 0 && i != 0 {
			t.Fatal("Block has no miner payouts")
		}
		if cbig.Timestamp == types.Timestamp(0) {
			t.Fatal("Timestamp wasn't set correctly")
		}
		if len(cbig.Transactions) == 0 {
			t.Fatal("Block doesn't have any transactions even though it should")
		}

		// Verify IDs
		for _, tx := range cbhg.Transactions {
			// Building transaction of type Transaction to use as
			// comparison for ID creation
			txn := types.Transaction{
				SiacoinInputs:         tx.SiacoinInputs,
				FileContractRevisions: tx.FileContractRevisions,
				StorageProofs:         tx.StorageProofs,
				SiafundInputs:         tx.SiafundInputs,
				MinerFees:             tx.MinerFees,
				ArbitraryData:         tx.ArbitraryData,
				TransactionSignatures: tx.TransactionSignatures,
			}
			for _, sco := range tx.SiacoinOutputs {
				txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
					Value:      sco.Value,
					UnlockHash: sco.UnlockHash,
				})
			}
			for i, fc := range tx.FileContracts {
				txn.FileContracts = append(txn.FileContracts, types.FileContract{
					FileSize:       fc.FileSize,
					FileMerkleRoot: fc.FileMerkleRoot,
					WindowStart:    fc.WindowStart,
					WindowEnd:      fc.WindowEnd,
					Payout:         fc.Payout,
					UnlockHash:     fc.UnlockHash,
					RevisionNumber: fc.RevisionNumber,
				})
				for _, vp := range fc.ValidProofOutputs {
					txn.FileContracts[i].ValidProofOutputs = append(txn.FileContracts[i].ValidProofOutputs, types.SiacoinOutput{
						Value:      vp.Value,
						UnlockHash: vp.UnlockHash,
					})
				}
				for _, mp := range fc.MissedProofOutputs {
					txn.FileContracts[i].MissedProofOutputs = append(txn.FileContracts[i].MissedProofOutputs, types.SiacoinOutput{
						Value:      mp.Value,
						UnlockHash: mp.UnlockHash,
					})
				}
			}
			for _, sfo := range tx.SiafundOutputs {
				txn.SiafundOutputs = append(txn.SiafundOutputs, types.SiafundOutput{
					Value:      sfo.Value,
					UnlockHash: sfo.UnlockHash,
					ClaimStart: types.ZeroCurrency,
				})
			}

			// Verify SiacoinOutput IDs
			for i, sco := range tx.SiacoinOutputs {
				if sco.ID != txn.SiacoinOutputID(uint64(i)) {
					t.Fatalf("SiacoinOutputID not as expected, got %v expected %v", sco.ID, txn.SiacoinOutputID(uint64(i)))
				}
			}

			// FileContracts
			for i, fc := range tx.FileContracts {
				// Verify FileContract ID
				fcid := txn.FileContractID(uint64(i))
				if fc.ID != fcid {
					t.Fatalf("FileContract ID not as expected, got %v expected %v", fc.ID, fcid)
				}
				// Verify ValidProof IDs
				for j, vp := range fc.ValidProofOutputs {
					if vp.ID != fcid.StorageProofOutputID(types.ProofValid, uint64(j)) {
						t.Fatalf("File Contract ValidProofOutputID not as expected, got %v expected %v", vp.ID, fcid.StorageProofOutputID(types.ProofValid, uint64(j)))
					}
				}
				// Verify MissedProof IDs
				for j, mp := range fc.MissedProofOutputs {
					if mp.ID != fcid.StorageProofOutputID(types.ProofMissed, uint64(j)) {
						t.Fatalf("File Contract MissedProofOutputID not as expected, got %v expected %v", mp.ID, fcid.StorageProofOutputID(types.ProofMissed, uint64(j)))
					}
				}
			}

			// Verify SiafundOutput IDs
			for i, sfo := range tx.SiafundOutputs {
				// Failing, switch back to !=
				if sfo.ID != txn.SiafundOutputID(uint64(i)) {
					t.Fatalf("SiafundOutputID not as expected, got %v expected %v", sfo.ID, txn.SiafundOutputID(uint64(i)))
				}
			}
		}
	}
}
