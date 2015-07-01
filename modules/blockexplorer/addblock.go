package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// appendAddress appends a modification object to conditionally
// appends an address to the stored address in a database
func appendAddress(modifications []persist.BoltModification, address types.UnlockHash, txid crypto.Hash) []persist.BoltModification {
	addr := encoding.Marshal(address)
	mapFunc := func(addrBytes []byte) (persist.BoltItem, error) {
		if addrBytes == nil {
			return persist.BoltItem{
				BucketName: "Addresses",
				Key:        addr,
				Value:      encoding.Marshal([]crypto.Hash{txid}),
			}, nil
		}

		var txids []crypto.Hash
		err := encoding.Unmarshal(addrBytes, &txids)
		if err != nil {
			return persist.BoltItem{}, err
		}

		// Don't append if the address is already in there
		var present bool = false
		for _, tx := range txids {
			if tx == txid {
				present = true
			}
		}
		if !present {
			txids = append(txids, txid)
		}

		return persist.BoltItem{
			BucketName: "Addresses",
			Key:        addr,
			Value:      encoding.Marshal(txids),
		}, nil
	}
	return append(modifications, persist.BoltModification{
		BucketName: "Addresses",
		Key:        addr,
		Map:        mapFunc,
	})
}

// appendSiacoinInput appends a modificatoin object to look up a
// siacoin output and set the input field
func appendSiacoinInput(modifications []persist.BoltModification, outputID types.SiacoinOutputID, txid crypto.Hash) []persist.BoltModification {
	oID := encoding.Marshal(outputID)
	mapFunc := func(outputBytes []byte) (persist.BoltItem, error) {
		if outputBytes == nil {
			return persist.BoltItem{}, errors.New("item not found in bucket")
		}
		var output outputTransactions
		err := encoding.Unmarshal(outputBytes, &output)
		if err != nil {
			return persist.BoltItem{}, err
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

// append SiafundInput does the same thing as appendSiacoinInput, but
// with siafunds instead
func appendSiafundInput(modifications []persist.BoltModification, outputID types.SiafundOutputID, txid crypto.Hash) []persist.BoltModification {
	oID := encoding.Marshal(outputID)
	mapFunc := func(outputBytes []byte) (persist.BoltItem, error) {
		if outputBytes == nil {
			return persist.BoltItem{}, errors.New("item not found in bucket")
		}
		var output outputTransactions
		err := encoding.Unmarshal(outputBytes, &output)
		if err != nil {
			return persist.BoltItem{}, err
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

// appendFcRevision appends a file contract revision to the list of
// revisions in a file contract
func appendFcRevision(modifications []persist.BoltModification, fcid types.FileContractID, txid crypto.Hash) []persist.BoltModification {
	fcidBytes := encoding.Marshal(fcid)
	mapFunc := func(fcBytes []byte) (persist.BoltItem, error) {
		if fcBytes == nil {
			return persist.BoltItem{}, errors.New("item not found in bucket")
		}
		var fcInfoStruct fcInfo
		err := encoding.Unmarshal(fcBytes, &fcInfoStruct)
		if err != nil {
			return persist.BoltItem{}, err
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

// appendFcProof looks up a file contract and appends the file proof
// to the filecontract object in the database
func appendFcProof(modifications []persist.BoltModification, fcid types.FileContractID, txid crypto.Hash) []persist.BoltModification {
	fcidBytes := encoding.Marshal(fcid)
	mapFunc := func(fcBytes []byte) (persist.BoltItem, error) {
		if fcBytes == nil {
			return persist.BoltItem{}, errors.New("item not found in bucket")
		}
		var fcInfoStruct fcInfo
		err := encoding.Unmarshal(fcBytes, &fcInfoStruct)
		if err != nil {
			return persist.BoltItem{}, err
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

// appendNewOutput returns a new additions list with the a bolt item
// for inserting the output id appended to the end
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

// appendNewSFOutput is the same as appendNewOutput, but for Siafunds
// instead of siacoins
func appendNewSFOutput(additions []persist.BoltItem, outputID types.SiafundOutputID, txid crypto.Hash) []persist.BoltItem {
	additions = appendHashType(additions, crypto.Hash(outputID), hashFundOutputID)
	return append(additions, persist.BoltItem{
		BucketName: "SiafundOutputs",
		Key:        encoding.Marshal(outputID),
		Value: encoding.Marshal(outputTransactions{
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

// Parses and adds
func (be *BlockExplorer) addBlockDB(b types.Block) error {
	// Special case for the genesis block, which does not have a
	// valid parent, and for testing, as tests will not always use
	// blocks in consensus
	var blocktarget types.Target
	if b.ID() == be.genesisBlockID {
		blocktarget = types.RootDepth
	} else {
		var exists bool
		blocktarget, exists = be.cs.ChildTarget(b.ParentID)
		if build.DEBUG {
			if build.Release == "testing" {
				blocktarget = types.RootDepth
			}
			if !exists {
				panic("Applied block not in consensus")
			}

		}
	}

	// Create an additions slice for this block
	var modifications []persist.BoltModification
	var additions []persist.BoltItem

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

	bSum := modules.ExplorerBlockData{
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

	// Add the block to the Transactions bucket so that lookups on
	// the block as a txid result in a special case, not an
	// error. Necessary because of miner payouts
	additions = append(additions, persist.BoltItem{
		BucketName: "Transactions",
		Key:        encoding.Marshal(b.ID()),
		Value:      encoding.Marshal(txInfo{b.ID(), -1}),
	})
	// TODO add a dummy txid with block id and txNum of -1, so
	// that txid lookups on a block id are still valid

	// Insert the miner payouts as new outputs
	for i := range b.MinerPayouts {
		modifications = appendAddress(modifications, b.MinerPayouts[i].UnlockHash, crypto.Hash(b.ID()))
		additions = appendHashType(additions, crypto.Hash(b.MinerPayouts[i].UnlockHash), hashUnlockHash)
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

	// Can put this in addBlockDB() to reduce the number of
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
		modifications = appendAddress(modifications, tx.SiacoinOutputs[i].UnlockHash, txid)
		changes = appendHashType(changes, crypto.Hash(tx.SiacoinOutputs[i].UnlockHash), hashUnlockHash)
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
			modifications = appendAddress(modifications, contract.ValidProofOutputs[i].UnlockHash, txid)
			changes = appendHashType(changes, crypto.Hash(contract.ValidProofOutputs[i].UnlockHash), hashUnlockHash)
		}
		for j := range contract.MissedProofOutputs {
			changes = appendNewOutput(changes, fcid.StorageProofOutputID(false, j), txid)
			modifications = appendAddress(modifications, contract.MissedProofOutputs[i].UnlockHash, txid)
			changes = appendHashType(changes, crypto.Hash(contract.MissedProofOutputs[i].UnlockHash), hashUnlockHash)
		}

		modifications = appendAddress(modifications, contract.UnlockHash, txid)
		changes = appendHashType(changes, crypto.Hash(contract.UnlockHash), hashUnlockHash)

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
			modifications = appendAddress(modifications, revision.NewValidProofOutputs[i].UnlockHash, txid)
			changes = appendHashType(changes, crypto.Hash(revision.NewValidProofOutputs[i].UnlockHash), hashUnlockHash)
		}
		for i := range revision.NewMissedProofOutputs {
			changes = appendNewOutput(changes, revision.ParentID.StorageProofOutputID(false, i), txid)
			modifications = appendAddress(modifications, revision.NewMissedProofOutputs[i].UnlockHash, txid)
			changes = appendHashType(changes, crypto.Hash(revision.NewMissedProofOutputs[i].UnlockHash), hashUnlockHash)
		}

		modifications = appendAddress(modifications, revision.NewUnlockHash, txid)
		changes = appendHashType(changes, crypto.Hash(revision.NewUnlockHash), hashUnlockHash)
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
		modifications = appendAddress(modifications, tx.SiafundOutputs[i].UnlockHash, txid)
		changes = appendHashType(changes, crypto.Hash(tx.SiafundOutputs[i].UnlockHash), hashUnlockHash)
	}

	changes = appendHashType(changes, txid, hashTransaction)

	err := be.db.BulkUpdate(modifications, changes)

	return err
}
