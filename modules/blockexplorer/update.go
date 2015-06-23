package blockexplorer

import (
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// Handles updates recieved from the consensus subscription. Keeps
// track of transaction volume, block timestamps and block sizes, as
// well as the current block height
func (be *BlockExplorer) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := be.mu.Lock()
	defer be.mu.Unlock(lockID)

	// Modify the number of file contracts and how much they costed
	for _, diff := range cc.FileContractDiffs {
		if diff.Direction == modules.DiffApply {
			be.activeContracts += 1
			be.totalContracts += 1
			be.activeContractCost = be.activeContractCost.Add(diff.FileContract.Payout)
			be.totalContractCost = be.totalContractCost.Add(diff.FileContract.Payout)
			be.activeContractSize += diff.FileContract.FileSize
			be.totalContractSize += diff.FileContract.FileSize
		} else {
			be.activeContracts -= 1
			be.activeContractCost = be.activeContractCost.Sub(diff.FileContract.Payout)
			be.activeContractSize -= diff.FileContract.FileSize
		}
	}

	// Reverting the blockheight and block data structs from reverted blocks
	be.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	be.blockSummaries = be.blockSummaries[:len(be.blockSummaries)-len(cc.RevertedBlocks)]

	// Handle incoming blocks
	for _, block := range cc.AppliedBlocks {
		// Special case for the genesis block, as it does not
		// have a valid parent id.
		var blocktarget types.Target
		if block.ID() == be.genesisBlockID {
			blocktarget = types.RootDepth
		} else {
			var exists bool
			blocktarget, exists = be.cs.ChildTarget(block.ParentID)
			if build.DEBUG {
				if !exists {
					panic("Applied block not in consensus")
				}
			}
		}

		// Marshall is used to get an exact byte size of the block
		be.blockSummaries = append(be.blockSummaries, modules.ExplorerBlockData{
			Timestamp: block.Timestamp,
			Target:    blocktarget,
			Size:      uint64(len(encoding.Marshal(block))),
		})

		err := be.addBlock(block)
		if err != nil {
			fmt.Printf("Error when adding block to database: " + err.Error() + "\n")
		}
		be.blockchainHeight += 1
	}
	be.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]

	// Notify subscribers about updates
	be.updateSubscribers()
}

func (be *BlockExplorer) addBlock(b types.Block) error {
	// Special case for the genesis block, which does not have a valid parent
	var blocktarget types.Target
	if b.ID() == be.genesisBlockID {
		blocktarget = types.RootDepth
	} else {
		var exists bool
		blocktarget, exists = be.cs.ChildTarget(b.ParentID)
		if build.DEBUG {
			if !exists {
				panic("Applied block not in consensus")
			}
		}
	}

	// Construct the struct that will be inside the database
	blockStruct := blockData{
		Block:  b,
		Height: be.blockchainHeight,
	}

	bSum := blockSummary{
		ID:        b.ID(),
		Timestamp: b.Timestamp,
		Target:    blocktarget,
		Size:      uint64(len(encoding.Marshal(b))),
	}

	err := be.db.InsertIntoBucket("Blocks", encoding.Marshal(b.ID()), encoding.Marshal(blockStruct))
	if err != nil {
		return err
	}
	err = be.db.InsertIntoBucket("Heights", encoding.Marshal(be.blockchainHeight), encoding.Marshal(bSum))
	if err != nil {
		return err
	}

	// Insert the miner payouts as new outputs
	changes := make([]persist.BoltItem, len(b.MinerPayouts))
	for i := range b.MinerPayouts {
		changes[i] = persist.BoltItem{
			BucketName: "SiacoinOutputs",
			Key:        encoding.Marshal(b.MinerPayoutID(i)),
			Value: encoding.Marshal(outputTransactions{
				OutputTx: crypto.Hash(b.ID()),
			}),
		}
	}
	err = be.db.BulkInsert(changes)
	if err != nil {
		return err
	}

	// Insert each transaction
	for i, tx := range b.Transactions {
		err = be.addTransaction(tx, b.ID(), i)
		if err != nil {
			return err
		}
	}
	return nil
}

func (be *BlockExplorer) addTransaction(tx types.Transaction, bID types.BlockID, txNum int) error {
	// Store this for quick lookup
	txid := tx.ID()

	// A list of things to be added to the database
	changes := make([]persist.BoltItem, 0)

	// Can put this in addBlock() to reduce the number of
	// parameters to this function, but conceptually it fits here
	// better
	changes = append(changes, persist.BoltItem{
		BucketName: "Transactions",
		Key:        encoding.Marshal(txid),
		Value:      encoding.Marshal(txInfo{bID, txNum}),
	})

	// Need to modify the outputs that inputs use, which requires
	// looking them up first
	inputRequests := make([]persist.BoltItem, len(tx.SiacoinInputs))
	for i, input := range tx.SiacoinInputs {
		inputRequests[i] = persist.BoltItem{"SiacoinOutputs", encoding.Marshal(input.ParentID), nil}
	}
	inputsBytes, err := be.db.BulkGet(inputRequests)
	if err != nil {
		return err
	}

	inputOutputs := make([]outputTransactions, len(inputsBytes))
	for i := range inputsBytes {
		err = encoding.Unmarshal(inputsBytes[i], &inputOutputs[i])
		if err != nil {
			return err
		}
		inputOutputs[i].InputTx = txid
		changes = append(changes, persist.BoltItem{
			BucketName: "SiacoinOutputs",
			Key:        encoding.Marshal(tx.SiacoinInputs[i].ParentID),
			Value:      encoding.Marshal(inputOutputs[i]),
		})
	}

	// Handle all the transaction outputs
	for i := range tx.SiacoinOutputs {
		changes = append(changes, persist.BoltItem{
			BucketName: "SiacoinOutputs",
			Key:        encoding.Marshal(tx.SiacoinOutputID(i)),
			Value: encoding.Marshal(outputTransactions{
				// The inputTx field is left blank as
				// the default
				OutputTx: txid,
			}),
		})
	}

	be.db.BulkInsert(changes)

	return nil
}
