package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/boltdb/bolt"
)

// A boltTx is a bolt transaction. It implements monadic error handling, such that
// any operation that occurs after an error becomes a no-op.
type boltTx struct {
	*bolt.Tx
	err error
}

func newBoltTx(db *explorerDB) (*boltTx, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	return &boltTx{tx, nil}, nil
}

func (tx *boltTx) commit() error {
	if tx.err != nil {
		return tx.err
	}
	return tx.Commit()
}

func (tx *boltTx) getObject(bucket string, key, obj interface{}) {
	// if an error has already be encountered, do nothing
	if tx.err != nil {
		return
	}

	b := tx.Bucket([]byte(bucket))
	if b == nil {
		tx.err = errors.New("bucket does not exist: " + bucket)
		return
	}
	objBytes := b.Get(encoding.Marshal(key))
	if objBytes == nil {
		tx.err = persist.ErrNilEntry
		return
	}
	tx.err = encoding.Unmarshal(objBytes, obj)
	return
}

func (tx *boltTx) putObject(bucket string, key, val interface{}) {
	// if an error has already be encountered, do nothing
	if tx.err != nil {
		return
	}

	b := tx.Bucket([]byte(bucket))
	if b == nil {
		tx.err = errors.New("bucket does not exist: " + bucket)
		return
	}
	tx.err = b.Put(encoding.Marshal(key), encoding.Marshal(val))
	return
}

// addAddress either creates a new list of transactions for the given
// address, or adds the txid to the list if such a list already exists
func (tx *boltTx) addAddress(addr types.UnlockHash, txid crypto.Hash) {
	tx.putObject("Hashes", crypto.Hash(addr), hashUnlockHash)

	oldErr := tx.err
	var txns []crypto.Hash
	tx.getObject("Addresses", addr, &txns)
	if oldErr == nil && tx.err == persist.ErrNilEntry {
		// NOTE: this is a special case where a nil entry is not an error, so
		// we must explicitly reset tx.err.
		tx.err = nil
	}
	txns = append(txns, txid)

	tx.putObject("Addresses", addr, txns)
}

// addSiacoinInput changes an existing outputTransactions struct to
// point to the place where that output was used
func (tx *boltTx) addSiacoinInput(outputID types.SiacoinOutputID, txid crypto.Hash) {
	var ot outputTransactions
	tx.getObject("SiacoinOutputs", outputID, &ot)
	ot.InputTx = txid
	tx.putObject("SiacoinOutputs", outputID, ot)
}

// addSiafundInpt does the same thing as addSiacoinInput except with siafunds
func (tx *boltTx) addSiafundInput(outputID types.SiafundOutputID, txid crypto.Hash) {
	var ot outputTransactions
	tx.getObject("SiafundOutputs", outputID, &ot)
	ot.InputTx = txid
	tx.putObject("SiafundOutputs", outputID, ot)
}

// addFcRevision changes an existing fcInfo struct to contain the txid
// of the contract revision
func (tx *boltTx) addFcRevision(fcid types.FileContractID, txid crypto.Hash) {
	var fi fcInfo
	tx.getObject("FileContracts", fcid, &fi)
	fi.Revisions = append(fi.Revisions, txid)
	tx.putObject("FileContracts", fcid, fi)
}

// addFcProof changes an existing fcInfo struct in the database to
// contain the txid of its storage proof
func (tx *boltTx) addFcProof(fcid types.FileContractID, txid crypto.Hash) {
	var fi fcInfo
	tx.getObject("FileContracts", fcid, &fi)
	fi.Proof = txid
	tx.putObject("FileContracts", fcid, fi)
}

func (tx *boltTx) addNewHash(bucketName string, t int, hash crypto.Hash, value interface{}) {
	tx.putObject("Hashes", hash, t)
	tx.putObject(bucketName, hash, value)
}

// addNewOutput creats a new outputTransactions struct and adds it to the database
func (tx *boltTx) addNewOutput(outputID types.SiacoinOutputID, txid crypto.Hash) {
	otx := outputTransactions{txid, crypto.Hash{}}
	tx.addNewHash("SiacoinOutputs", hashCoinOutputID, crypto.Hash(outputID), otx)
}

// addNewSFOutput does the same thing as addNewOutput does, except for siafunds
func (tx *boltTx) addNewSFOutput(outputID types.SiafundOutputID, txid crypto.Hash) {
	otx := outputTransactions{txid, crypto.Hash{}}
	tx.addNewHash("SiafundOutputs", hashFundOutputID, crypto.Hash(outputID), otx)
}

// addBlockDB parses a block and adds it to the database
func (be *BlockExplorer) addBlockDB(b types.Block) error {
	// Special case for the genesis block, which does not have a
	// valid parent, and for testing, as tests will not always use
	// blocks in consensus
	var blocktarget types.Target
	if b.ID() == be.genesisBlockID {
		blocktarget = types.RootDepth
		be.blockchainHeight = 0
	} else {
		var exists bool
		blocktarget, exists = be.cs.ChildTarget(b.ParentID)
		if build.DEBUG {
			if build.Release == "testing" {
				blocktarget = types.RootDepth
			} else if !exists {
				panic("Applied block not in consensus")
			}

		}
	}

	// Check if the block exists
	exists, err := be.db.Exists("Blocks", encoding.Marshal(b.ID()))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	tx, err := newBoltTx(be.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Construct the struct that will be inside the heights map
	blockStruct := blockData{
		Block:  b,
		Height: be.blockchainHeight,
	}

	tx.addNewHash("Blocks", hashBlock, crypto.Hash(b.ID()), blockStruct)

	bSum := modules.ExplorerBlockData{
		ID:        b.ID(),
		Timestamp: b.Timestamp,
		Target:    blocktarget,
		Size:      uint64(len(encoding.Marshal(b))),
	}

	tx.putObject("Heights", be.blockchainHeight, bSum)
	tx.putObject("Hashes", crypto.Hash(b.ID()), hashBlock)

	// Insert the miner payouts as new outputs
	for i, payout := range b.MinerPayouts {
		tx.addAddress(payout.UnlockHash, crypto.Hash(b.ID()))
		tx.addNewOutput(b.MinerPayoutID(i), crypto.Hash(b.ID()))
	}

	// Insert each transaction
	for i, txn := range b.Transactions {
		tx.addNewHash("Transactions", hashTransaction, txn.ID(), txInfo{b.ID(), i})
		tx.addTransaction(txn)
	}

	return tx.commit()
}

// addTransaction is called from addBlockDB, and delegates the adding
// of information to the database to the functions defined above
func (tx *boltTx) addTransaction(txn types.Transaction) {
	// Store this for quick lookup
	txid := txn.ID()

	// Append each input to the list of modifications
	for _, input := range txn.SiacoinInputs {
		tx.addSiacoinInput(input.ParentID, txid)
	}

	// Handle all the transaction outputs
	for i, output := range txn.SiacoinOutputs {
		tx.addAddress(output.UnlockHash, txid)
		tx.addNewOutput(txn.SiacoinOutputID(i), txid)
	}

	// Handle each file contract individually
	for i, contract := range txn.FileContracts {
		fcid := txn.FileContractID(i)
		tx.addNewHash("FileContracts", hashFilecontract, crypto.Hash(fcid), fcInfo{
			Contract: txid,
		})

		for j, output := range contract.ValidProofOutputs {
			tx.addAddress(output.UnlockHash, txid)
			tx.addNewOutput(fcid.StorageProofOutputID(true, j), txid)
		}
		for j, output := range contract.MissedProofOutputs {
			tx.addAddress(output.UnlockHash, txid)
			tx.addNewOutput(fcid.StorageProofOutputID(false, j), txid)
		}

		tx.addAddress(contract.UnlockHash, txid)
	}

	// Update the list of revisions
	for _, revision := range txn.FileContractRevisions {
		tx.addFcRevision(revision.ParentID, txid)

		// Note the old outputs will still be there in the
		// database. This is to provide information to the
		// people who may just need it.
		for i, output := range revision.NewValidProofOutputs {
			tx.addAddress(output.UnlockHash, txid)
			tx.addNewOutput(revision.ParentID.StorageProofOutputID(true, i), txid)
		}
		for i, output := range revision.NewMissedProofOutputs {
			tx.addAddress(output.UnlockHash, txid)
			tx.addNewOutput(revision.ParentID.StorageProofOutputID(false, i), txid)
		}

		tx.addAddress(revision.NewUnlockHash, txid)
	}

	// Update the list of storage proofs
	for _, proof := range txn.StorageProofs {
		tx.addFcProof(proof.ParentID, txid)
	}

	// Append all the siafund inputs to the modification list
	for _, input := range txn.SiafundInputs {
		tx.addSiafundInput(input.ParentID, txid)
	}

	// Handle all the siafund outputs
	for i, output := range txn.SiafundOutputs {
		tx.addAddress(output.UnlockHash, txid)
		tx.addNewSFOutput(txn.SiafundOutputID(i), txid)

	}

	tx.putObject("Hashes", txid, hashTransaction)
}
