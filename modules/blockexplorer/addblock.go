package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/boltdb/bolt"
)

var (
	ErrNilEntry = errors.New("entry does not exist")
)

func getObject(tx *bolt.Tx, bucket string, key, obj interface{}) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return errors.New("bucket does not exist: " + bucket)
	}
	objBytes := b.Get(encoding.Marshal(key))
	if objBytes == nil {
		return ErrNilEntry
	}
	return encoding.Unmarshal(objBytes, obj)
}

func putObject(tx *bolt.Tx, bucket string, key, val interface{}) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return errors.New("bucket does not exist: " + bucket)
	}
	return b.Put(encoding.Marshal(key), encoding.Marshal(val))
}

// addHashType adds an entry in the Hashes bucket for identifing that hash
func addHashType(tx *bolt.Tx, hash crypto.Hash, hashType int) error {
	return putObject(tx, "Hashes", hash, hashType)
}

// addAddress either creates a new list of transactions for the given
// address, or adds the txid to the list if such a list already exists
func addAddress(tx *bolt.Tx, addr types.UnlockHash, txid crypto.Hash) error {
	err := putObject(tx, "Hashes", crypto.Hash(addr), hashUnlockHash)
	if err != nil {
		return err
	}

	var txns []crypto.Hash
	err = getObject(tx, "Addresses", addr, &txns)
	if err != ErrNilEntry {
		return err
	}
	txns = append(txns, txid)

	return putObject(tx, "Addresses", addr, txns)
}

// addSiacoinInput changes an existing outputTransactions struct to
// point to the place where that output was used
func addSiacoinInput(tx *bolt.Tx, outputID types.SiacoinOutputID, txid crypto.Hash) error {
	var ot outputTransactions
	err := getObject(tx, "SiacoinOutputs", outputID, &ot)
	if err != nil {
		return err
	}

	ot.InputTx = txid

	return putObject(tx, "SiacoinOutputs", outputID, ot)
}

// addSiafundInpt does the same thing as addSiacoinInput except with siafunds
func addSiafundInput(tx *bolt.Tx, outputID types.SiafundOutputID, txid crypto.Hash) error {
	var ot outputTransactions
	err := getObject(tx, "SiafundOutputs", outputID, &ot)
	if err != nil {
		return err
	}

	ot.InputTx = txid

	return putObject(tx, "SiafundOutputs", outputID, ot)
}

// addFcRevision changes an existing fcInfo struct to contain the txid
// of the contract revision
func addFcRevision(tx *bolt.Tx, fcid types.FileContractID, txid crypto.Hash) error {
	var fi fcInfo
	err := getObject(tx, "FileContracts", fcid, &fi)
	if err != nil {
		return err
	}

	fi.Revisions = append(fi.Revisions, txid)

	return putObject(tx, "FileContracts", fcid, fi)
}

// addFcProof changes an existing fcInfo struct in the database to
// contain the txid of its storage proof
func addFcProof(tx *bolt.Tx, fcid types.FileContractID, txid crypto.Hash) error {
	var fi fcInfo
	err := getObject(tx, "FileContracts", fcid, &fi)
	if err != nil {
		return err
	}

	fi.Proof = txid

	return putObject(tx, "FileContracts", fcid, fi)
}

func addNewHash(tx *bolt.Tx, bucketName string, t int, hash crypto.Hash, value interface{}) error {
	err := putObject(tx, "Hashes", hash, t)
	if err != nil {
		return err
	}
	return putObject(tx, bucketName, hash, value)
}

// addNewOutput creats a new outputTransactions struct and adds it to the database
func addNewOutput(tx *bolt.Tx, outputID types.SiacoinOutputID, txid crypto.Hash) error {
	otx := outputTransactions{txid, crypto.Hash{}}
	return addNewHash(tx, "SiacoinOutputs", hashCoinOutputID, crypto.Hash(outputID), otx)
}

// addNewSFOutput does the same thing as addNewOutput does, except for siafunds
func addNewSFOutput(tx *bolt.Tx, outputID types.SiafundOutputID, txid crypto.Hash) error {
	otx := outputTransactions{txid, crypto.Hash{}}
	return addNewHash(tx, "SiafundOutputs", hashFundOutputID, crypto.Hash(outputID), otx)
}

// addBlockDB parses a block and adds it to the database
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

	tx, err := be.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Construct the struct that will be inside the database
	blockStruct := blockData{
		Block:  b,
		Height: be.blockchainHeight,
	}

	err = addNewHash(tx, "Blocks", hashBlock, crypto.Hash(b.ID()), blockStruct)
	if err != nil {
		return err
	}

	bSum := modules.ExplorerBlockData{
		ID:        b.ID(),
		Timestamp: b.Timestamp,
		Target:    blocktarget,
		Size:      uint64(len(encoding.Marshal(b))),
	}

	err = putObject(tx, "Heights", be.blockchainHeight, bSum)
	if err != nil {
		return err
	}
	err = putObject(tx, "Hashes", crypto.Hash(b.ID()), hashBlock)
	if err != nil {
		return err
	}

	// Insert the miner payouts as new outputs
	for i, payout := range b.MinerPayouts {
		err = addAddress(tx, payout.UnlockHash, crypto.Hash(b.ID()))
		if err != nil {
			return err
		}
		err = addNewOutput(tx, b.MinerPayoutID(i), crypto.Hash(b.ID()))
		if err != nil {
			return err
		}
	}

	// Insert each transaction
	for i, txn := range b.Transactions {
		err = addNewHash(tx, "Transactions", hashTransaction, txn.ID(), txInfo{b.ID(), i})
		if err != nil {
			return err
		}
		err = be.addTransaction(tx, txn)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// addTransaction is called from addBlockDB, and delegates the adding
// of information to the database to the functions defined above
func (be *BlockExplorer) addTransaction(btx *bolt.Tx, tx types.Transaction) error {
	// Store this for quick lookup
	txid := tx.ID()

	// Append each input to the list of modifications
	for _, input := range tx.SiacoinInputs {
		err := addSiacoinInput(btx, input.ParentID, txid)
		if err != nil {
			return err
		}
	}

	// Handle all the transaction outputs
	for i, output := range tx.SiacoinOutputs {
		err := addAddress(btx, output.UnlockHash, txid)
		if err != nil {
			return err
		}
		err = addNewOutput(btx, tx.SiacoinOutputID(i), txid)
		if err != nil {
			return err
		}
	}

	// Handle each file contract individually
	for i, contract := range tx.FileContracts {
		fcid := tx.FileContractID(i)
		err := addNewHash(btx, "FileContracts", hashFilecontract, crypto.Hash(fcid), fcInfo{
			Contract: txid,
		})
		if err != nil {
			return err
		}

		for j, output := range contract.ValidProofOutputs {
			err = addAddress(btx, output.UnlockHash, txid)
			if err != nil {
				return err
			}
			err = addNewOutput(btx, fcid.StorageProofOutputID(true, j), txid)
			if err != nil {
				return err
			}
		}
		for j, output := range contract.MissedProofOutputs {
			err = addAddress(btx, output.UnlockHash, txid)
			if err != nil {
				return err
			}
			err = addNewOutput(btx, fcid.StorageProofOutputID(false, j), txid)
			if err != nil {
				return err
			}
		}

		err = addAddress(btx, contract.UnlockHash, txid)
		if err != nil {
			return err
		}
	}

	// Update the list of revisions
	for _, revision := range tx.FileContractRevisions {
		err := addFcRevision(btx, revision.ParentID, txid)
		if err != nil {
			return err
		}

		// Note the old outputs will still be there in the
		// database. This is to provide information to the
		// people who may just need it.
		for i, output := range revision.NewValidProofOutputs {
			err = addAddress(btx, output.UnlockHash, txid)
			if err != nil {
				return err
			}
			err = addNewOutput(btx, revision.ParentID.StorageProofOutputID(true, i), txid)
			if err != nil {
				return err
			}
		}
		for i, output := range revision.NewMissedProofOutputs {
			err = addAddress(btx, output.UnlockHash, txid)
			if err != nil {
				return err
			}
			err = addNewOutput(btx, revision.ParentID.StorageProofOutputID(false, i), txid)
			if err != nil {
				return err
			}
		}

		addAddress(btx, revision.NewUnlockHash, txid)
	}

	// Update the list of storage proofs
	for _, proof := range tx.StorageProofs {
		err := addFcProof(btx, proof.ParentID, txid)
		if err != nil {
			return err
		}
	}

	// Append all the siafund inputs to the modification list
	for _, input := range tx.SiafundInputs {
		err := addSiafundInput(btx, input.ParentID, txid)
		if err != nil {
			return err
		}
	}

	// Handle all the siafund outputs
	for i, output := range tx.SiafundOutputs {
		err := addAddress(btx, output.UnlockHash, txid)
		if err != nil {
			return err
		}
		err = addNewSFOutput(btx, tx.SiafundOutputID(i), txid)
		if err != nil {
			return err
		}

	}

	return putObject(btx, "Hashes", txid, hashTransaction)
}
