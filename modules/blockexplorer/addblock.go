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

// addHashType adds an entry in the Hashes bucket for identifing that hash
func addHashType(tx *bolt.Tx, hash crypto.Hash, hashType int) error {
	b := tx.Bucket([]byte("Hashes"))
	if b == nil {
		return errors.New("bucket Hashes does not exist")
	}

	return b.Put(encoding.Marshal(hash), encoding.Marshal(hashType))
}

// addAddress either creates a new list of transactions for the given
// address, or adds the txid to the list if such a list already exists
func addAddress(tx *bolt.Tx, addr types.UnlockHash, txid crypto.Hash) error {
	err := addHashType(tx, crypto.Hash(addr), hashUnlockHash)
	if err != nil {
		return err
	}

	b := tx.Bucket([]byte("Addresses"))
	if b == nil {
		return errors.New("Addresses bucket does not exist")
	}

	txBytes := b.Get(encoding.Marshal(addr))
	if txBytes == nil {
		err := b.Put(encoding.Marshal(addr), encoding.Marshal([]crypto.Hash{txid}))
		if err != nil {
			return err
		}
		return nil
	}

	var txns []crypto.Hash
	err = encoding.Unmarshal(txBytes, &txns)
	if err != nil {
		return err
	}

	txns = append(txns, txid)

	return b.Put(encoding.Marshal(addr), encoding.Marshal(txns))
}

// addSiacoinInput changes an existing outputTransactions struct to
// point to the place where that output was used
func addSiacoinInput(tx *bolt.Tx, outputID types.SiacoinOutputID, txid crypto.Hash) error {
	b := tx.Bucket([]byte("SiacoinOutputs"))
	if b == nil {
		return errors.New("bucket SiacoinOutputs does not exist")
	}

	outputBytes := b.Get(encoding.Marshal(outputID))
	if outputBytes == nil {
		return errors.New("output for id does not exist")
	}

	var ot outputTransactions
	err := encoding.Unmarshal(outputBytes, &ot)
	if err != nil {
		return err
	}

	ot.InputTx = txid

	return b.Put(encoding.Marshal(outputID), encoding.Marshal(ot))
}

// addSiafundInpt does the same thing as addSiacoinInput except with siafunds
func addSiafundInput(tx *bolt.Tx, outputID types.SiafundOutputID, txid crypto.Hash) error {
	b := tx.Bucket([]byte("SiafundOutputs"))
	if b == nil {
		return errors.New("bucket SaifundOutputs does not exist")
	}

	outputBytes := b.Get(encoding.Marshal(outputID))
	if outputBytes == nil {
		return errors.New("output for id does not exist")
	}

	var ot outputTransactions
	err := encoding.Unmarshal(outputBytes, &ot)
	if err != nil {
		return err
	}

	ot.InputTx = txid

	return b.Put(encoding.Marshal(outputID), encoding.Marshal(ot))
}

// addFcRevision changes an existing fcInfo struct to contain the txid
// of the contract revision
func addFcRevision(tx *bolt.Tx, fcid types.FileContractID, txid crypto.Hash) error {
	b := tx.Bucket([]byte("FileContracts"))
	if b == nil {
		return errors.New("bucket FileContracts does not exist")
	}

	fiBytes := b.Get(encoding.Marshal(fcid))
	if fiBytes == nil {
		return errors.New("filecontract does not exist in database")
	}

	var fi fcInfo
	err := encoding.Unmarshal(fiBytes, &fi)
	if err != nil {
		return err
	}

	fi.Revisions = append(fi.Revisions, txid)

	return b.Put(encoding.Marshal(fcid), encoding.Marshal(fi))
}

// addFcProof changes an existing fcInfo struct in the database to
// contain the txid of its storage proof
func addFcProof(tx *bolt.Tx, fcid types.FileContractID, txid crypto.Hash) error {
	b := tx.Bucket([]byte("FileContracts"))
	if b == nil {
		return errors.New("bucket FileContracts does not exist")
	}

	fiBytes := b.Get(encoding.Marshal(fcid))
	if fiBytes == nil {
		return errors.New("filecontract does not exist in database")
	}

	var fi fcInfo
	err := encoding.Unmarshal(fiBytes, &fi)
	if err != nil {
		return err
	}

	fi.Proof = txid

	return b.Put(encoding.Marshal(fcid), encoding.Marshal(fi))
}

// addNewOutput creats a new outputTransactions struct and adds it to the database
func addNewOutput(tx *bolt.Tx, outputID types.SiacoinOutputID, txid crypto.Hash) error {
	err := addHashType(tx, crypto.Hash(outputID), hashCoinOutputID)
	if err != nil {
		return err
	}

	b := tx.Bucket([]byte("SiacoinOutputs"))
	if b == nil {
		return errors.New("bucket SiacoinOutputs does not exist")
	}

	return b.Put(encoding.Marshal(outputID), encoding.Marshal(outputTransactions{
		OutputTx: txid,
	}))
}

// addNewSFOutput does the same thing as addNewOutput does, except for siafunds
func addNewSFOutput(tx *bolt.Tx, outputID types.SiafundOutputID, txid crypto.Hash) error {
	b := tx.Bucket([]byte("SiafundOutputs"))
	if b == nil {
		return errors.New("bucket SaifundOutputs does not exist")
	}

	return b.Put(encoding.Marshal(outputID), encoding.Marshal(outputTransactions{
		OutputTx: txid,
	}))
}

// addBlock creates a new blockData struct containing a block and adds
// it to the database
func addBlock(tx *bolt.Tx, id types.BlockID, bd blockData) error {
	b := tx.Bucket([]byte("Blocks"))
	if b == nil {
		return errors.New("bucket Blocks does not exist")
	}

	return b.Put(encoding.Marshal(id), encoding.Marshal(bd))
}

// addTxid creates a new txInfo struct and adds it to the database
func addTxid(tx *bolt.Tx, txid crypto.Hash, ti txInfo) error {
	err := addHashType(tx, txid, hashTransaction)
	if err != nil {
		return err
	}

	b := tx.Bucket([]byte("Transactions"))
	if b == nil {
		return errors.New("bucket Transactions does not exist")
	}

	return b.Put(encoding.Marshal(txid), encoding.Marshal(ti))
}

// addFcid creates a new fcInfo struct about a file contract and adds
// it to the database
func addFcid(tx *bolt.Tx, fcid types.FileContractID, fi fcInfo) error {
	err := addHashType(tx, crypto.Hash(fcid), hashFilecontract)
	if err != nil {
		return err
	}

	b := tx.Bucket([]byte("FileContracts"))
	if b == nil {
		return errors.New("bucket FileContracts does not exist")
	}

	return b.Put(encoding.Marshal(fcid), encoding.Marshal(fi))
}

// addHeight adds a block summary (modules.ExplorerBlockData) to the
// database with a height as the key
func addHeight(tx *bolt.Tx, height types.BlockHeight, bs modules.ExplorerBlockData) error {
	b := tx.Bucket([]byte("Heights"))
	if b == nil {
		return errors.New("bucket Blocks does not exist")
	}

	return b.Put(encoding.Marshal(height), encoding.Marshal(bs))
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

	err = addBlock(tx, b.ID(), blockStruct)
	if err != nil {
		return err
	}

	bSum := modules.ExplorerBlockData{
		ID:        b.ID(),
		Timestamp: b.Timestamp,
		Target:    blocktarget,
		Size:      uint64(len(encoding.Marshal(b))),
	}

	err = addHeight(tx, be.blockchainHeight, bSum)
	if err != nil {
		return err
	}
	err = addHashType(tx, crypto.Hash(b.ID()), hashBlock)
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
		err = addTxid(tx, txn.ID(), txInfo{b.ID(), i})
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
		err := addFcid(btx, fcid, fcInfo{
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

	return addHashType(btx, txid, hashTransaction)
}
