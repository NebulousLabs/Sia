package blockexplorer

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// func appendModification(modifications []persist.BoltModification, bucketName string, modID crypto.Hash, txid crypto.Hash, t reflect.Type, modFunc func(input t) t ) {
// 	modIDBytes = encoding.Marshal(modID)
// 	mapFunc := func(modStructBytes []byte) (presist.BoltItem, error) {
// 		if modStructBytes == nil {
// 			return errors.New(fmt.Sprintf("requested item %x does not exist in bucket %s", modIDBytes, bucketName))
// 		}
// 		var modStruct t
// 		encoding.Unmarshal(modStructBytes, modStructType)

// 	}
// }

func appendSiacoinInput(modifications []persist.BoltModification, outputID types.SiacoinOutputID, txid crypto.Hash) []persist.BoltModification {
	oID := encoding.Marshal(outputID)
	mapFunc := func(outputBytes []byte) (persist.BoltItem, error) {
		var output outputTransactions
		err := encoding.Unmarshal(outputBytes, &output)
		if err != nil {
			return *new(persist.BoltItem), err
		}

		output.InputTx = txid
		return persist.BoltItem{
			BucketName: "SiacoinOutputs",
			Key:        oID,
			Value:      encoding.Marshal(output),
		}, nil
	}
	return append(modifications, persist.BoltModification{
		BucketName: "SiacoinOutputs",
		Key:        oID,
		Map:        mapFunc,
	})
}

func appendSiafundInput(modifications []persist.BoltModification, outputID types.SiafundOutputID, txid crypto.Hash) []persist.BoltModification {
	oID := encoding.Marshal(outputID)
	mapFunc := func(outputBytes []byte) (persist.BoltItem, error) {
		var output outputTransactions
		err := encoding.Unmarshal(outputBytes, &output)
		if err != nil {
			return *new(persist.BoltItem), err
		}

		output.InputTx = txid
		return persist.BoltItem{
			BucketName: "SiafundOutputs",
			Key:        oID,
			Value:      encoding.Marshal(output),
		}, nil
	}
	return append(modifications, persist.BoltModification{
		BucketName: "SiafundOutputs",
		Key:        oID,
		Map:        mapFunc,
	})
}

func appendFcRevision(modifications []persist.BoltModification, fcid types.FileContractID, txid crypto.Hash) []persist.BoltModification {
	fcidBytes := encoding.Marshal(fcid)
	mapFunc := func(fcBytes []byte) (persist.BoltItem, error) {
		var fcInfoStruct fcInfo
		err := encoding.Unmarshal(fcBytes, &fcInfoStruct)
		if err != nil {
			return *new(persist.BoltItem), err
		}

		fcInfoStruct.Revisions = append(fcInfoStruct.Revisions, txid)
		return persist.BoltItem{
			BucketName: "FileContracts",
			Key:        fcidBytes,
			Value:      encoding.Marshal(fcInfoStruct),
		}, nil
	}
	return append(modifications, persist.BoltModification{
		BucketName: "FileContracts",
		Key:        fcidBytes,
		Map:        mapFunc,
	})
}

func appendFcProof(modifications []persist.BoltModification, fcid types.FileContractID, txid crypto.Hash) []persist.BoltModification {
	fcidBytes := encoding.Marshal(fcid)
	mapFunc := func(fcBytes []byte) (persist.BoltItem, error) {
		var fcInfoStruct fcInfo
		err := encoding.Unmarshal(fcBytes, &fcInfoStruct)
		if err != nil {
			return *new(persist.BoltItem), err
		}

		fcInfoStruct.Proof = txid
		return persist.BoltItem{
			BucketName: "FileContracts",
			Key:        fcidBytes,
			Value:      encoding.Marshal(fcInfoStruct),
		}, nil
	}
	return append(modifications, persist.BoltModification{
		BucketName: "FileContracts",
		Key:        fcidBytes,
		Map:        mapFunc,
	})
}

// returns a new additions list with the a bolt item for inserting the output id appended to the end
func appendNewOutput(additions []persist.BoltItem, outputID types.SiacoinOutputID, txid crypto.Hash) []persist.BoltItem {
	additions = appendHashType(additions, crypto.Hash(outputID), hashCoinOutputID)
	return append(additions, persist.BoltItem{
		BucketName: "SiacoinOutputs",
		Key:        encoding.Marshal(outputID),
		Value: encoding.Marshal(outputTransactions{
			OutputTx: txid,
		}),
	})
}

// Same as appendNewOutput, but for SiaFunds instead of siacoins
func appendNewSFOutput(additions []persist.BoltItem, outputID types.SiafundOutputID, txid crypto.Hash) []persist.BoltItem {
	additions = appendHashType(additions, crypto.Hash(outputID), hashFundOutputID)
	return append(additions, persist.BoltItem{
		BucketName: "SiafundOutputs",
		Key:        encoding.Marshal(outputID),
		Value:      encoding.Marshal(outputTransactions{
			OutputTx: txid,
		}),
	})
}

func appendHashType(changes []persist.BoltItem, hash crypto.Hash, hashType int) []persist.BoltItem {
	return append(changes, persist.BoltItem{
		BucketName: "Hashes",
		Key:        encoding.Marshal(hash),
		Value:      encoding.Marshal(hashType),
	})
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

	// Create an additions slice for this block
	modifications := make([]persist.BoltModification, 0)
	additions := make([]persist.BoltItem, 0)

	// Construct the struct that will be inside the database
	blockStruct := blockData{
		Block:  b,
		Height: be.blockchainHeight,
	}

	additions = append(additions, persist.BoltItem{
		BucketName: "Blocks",
		Key:        encoding.Marshal(b.ID()),
		Value:      encoding.Marshal(blockStruct),
	})

	bSum := blockSummary{
		ID:        b.ID(),
		Timestamp: b.Timestamp,
		Target:    blocktarget,
		Size:      uint64(len(encoding.Marshal(b))),
	}

	additions = append(additions, persist.BoltItem{
		BucketName: "Heights",
		Key:        encoding.Marshal(be.blockchainHeight),
		Value:      encoding.Marshal(bSum),
	})

	additions = appendHashType(additions, crypto.Hash(b.ID()), hashBlock)

	// Insert the miner payouts as new outputs
	for i := range b.MinerPayouts {
		additions = appendNewOutput(additions, b.MinerPayoutID(i), crypto.Hash(b.ID()))
	}

	err := be.db.BulkUpdate(modifications, additions)
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

	// A list of things to be added to the database. They will all
	// be performed at the end of this function
	modifications := make([]persist.BoltModification, 0)
	changes := make([]persist.BoltItem, 0)

	// Can put this in addBlock() to reduce the number of
	// parameters to this function, but conceptually it fits here
	// better
	changes = append(changes, persist.BoltItem{
		BucketName: "Transactions",
		Key:        encoding.Marshal(txid),
		Value:      encoding.Marshal(txInfo{bID, txNum}),
	})

	// Append each input to the list of modifications
	for _, input := range tx.SiacoinInputs {
		modifications = appendSiacoinInput(modifications, input.ParentID, txid)
	}

	// Handle all the transaction outputs
	for i := range tx.SiacoinOutputs {
		changes = appendNewOutput(changes, tx.SiacoinOutputID(i), txid)
	}

	// Handle each file contract individually
	for i, contract := range tx.FileContracts {
		fcid := tx.FileContractID(i)
		changes = append(changes, persist.BoltItem{
			BucketName: "FileContracts",
			Key:        encoding.Marshal(fcid),
			Value: encoding.Marshal(fcInfo{
				Contract: txid,
			}),
		})

		for j := range contract.ValidProofOutputs {
			changes = appendNewOutput(changes, fcid.StorageProofOutputID(true, j), txid)
		}
		for j := range contract.MissedProofOutputs {
			changes = appendNewOutput(changes, fcid.StorageProofOutputID(false, j), txid)
		}

		changes = appendHashType(changes, crypto.Hash(fcid), hashFilecontract)
	}

	// Update the list of revisions
	for _, revision := range tx.FileContractRevisions {
		modifications = appendFcRevision(modifications, revision.ParentID, txid)

		// Note the old outputs will still be there in the
		// database. This is to provide information to the
		// people who may just need it.
		for i := range revision.NewValidProofOutputs {
			changes = appendNewOutput(changes, revision.ParentID.StorageProofOutputID(true, i), txid)
		}
		for i := range revision.NewMissedProofOutputs {
			changes = appendNewOutput(changes, revision.ParentID.StorageProofOutputID(false, i), txid)
		}
	}

	// Update the list of storage proofs
	for _, proof := range tx.StorageProofs {
		modifications = appendFcProof(modifications, proof.ParentID, txid)
	}

	// Append all the siafund inputs to the modification list
	for _, input := range tx.SiafundInputs {
		modifications = appendSiafundInput(modifications, input.ParentID, txid)
	}

	// Handle all the siafund outputs
	for i := range tx.SiafundOutputs {
		changes = appendNewSFOutput(changes, tx.SiafundOutputID(i), txid)
	}

	changes = appendHashType(changes, txid, hashTransaction)

	err := be.db.BulkUpdate(modifications, changes)

	return err
}
