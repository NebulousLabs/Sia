package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// This type is almost the same as a block, except all the transactions are replaced with their transaction ID's, when just the rest of the block is needed. The ID is also included
type sparceBlock struct {
	ParentID     types.BlockID
	Nonce        types.BlockNonce
	Timestamp    types.Timestamp
	MinerPayouts []types.SiacoinOutput
	Transactions []crypto.Hash
	ID           types.BlockID
}

// GetHashInfo returns sufficient data about the hash that was
// provided to do more extensive lookups
func (be *BlockExplorer) GetHashInfo(hash crypto.Hash) (interface{}, error) {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	// Perform a lookup to tell which type of hash it is
	typeBytes, err := be.db.GetFromBucket("Hashes", hash[:])
	if err != nil {
		return nil, err
	}
	if typeBytes == nil {
		return nil, errors.New("requested hash not found in database")
	}

	var hashType int
	err = encoding.Unmarshal(typeBytes, &hashType)

	switch hashType {
	case hashBlock:
		var id types.BlockID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getBlock(types.BlockID(hash))
	case hashTransaction:
		var id crypto.Hash
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getTransaction(hash)
	case hashFilecontract:
		var id types.FileContractID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getFileContract(types.FileContractID(hash))
	case hashCoinOutputID:
		var id types.SiacoinOutputID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getSiacoinOutput(types.SiacoinOutputID(hash))
	case hashFundOutputID:
		var id types.SiafundOutputID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getSiafundOutput(types.SiafundOutputID(hash))
	case hashUnlockHash:
		var id types.UnlockHash
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getAddressTransactions(types.UnlockHash(hash))
	default:
		return nil, errors.New("bad hash type")
	}
}

// Returns the block with a given id
func (db *explorerDB) getBlock(id types.BlockID) (block types.Block, err error) {
	b, err := db.GetFromBucket("Blocks", encoding.Marshal(id))
	if err != nil {
		return block, err
	}

	err = encoding.Unmarshal(b, &block)
	if err != nil {
		return block, err
	}

	return block, nil
}

// Returns the transaction with the given id
func (db *explorerDB) getTransaction(id crypto.Hash) (types.Transaction, error) {
	var tx types.Transaction

	// Look up the transaction's location
	tBytes, err := db.GetFromBucket("Transactions", encoding.Marshal(id))
	if err != nil {
		return tx, err
	}

	var tLocation txInfo
	err = encoding.Unmarshal(tBytes, &tLocation)
	if err != nil {
		return tx, err
	}

	// Look up the block specified by the location and extract the transaction
	bBytes, err := db.GetFromBucket("Blocks", encoding.Marshal(tLocation.BlockID))
	if err != nil {
		return tx, err
	}

	var block types.Block
	err = encoding.Unmarshal(bBytes, &block)
	if err != nil {
		return tx, err
	}
	tx = block.Transactions[tLocation.TxNum]
	return tx, nil
}

// Returns the list of transactions a file contract with a given id has taken part in
func (db *explorerDB) getFileContract(id types.FileContractID) (fcInfo, error) {
	var fc fcInfo
	fcBytes, err := db.GetFromBucket("FileContracts", encoding.Marshal(id))
	if err != nil {
		return fc, err
	}

	err = encoding.Unmarshal(fcBytes, &fc)
	if err != nil {
		return fc, err
	}

	return fc, nil
}

func (db *explorerDB) getSiacoinOutput(id types.SiacoinOutputID) (outputTransactions, error) {
	var ot outputTransactions
	otBytes, err := db.GetFromBucket("SiacoinOutputs", encoding.Marshal(id))
	if err != nil {
		return ot, err
	}

	err = encoding.Unmarshal(otBytes, &ot)
	if err != nil {
		return ot, err
	}

	return ot, nil
}

func (db *explorerDB) getSiafundOutput(id types.SiafundOutputID) (outputTransactions, error) {
	var ot outputTransactions
	otBytes, err := db.GetFromBucket("SiafundOutputs", encoding.Marshal(id))
	if err != nil {
		return ot, err
	}

	err = encoding.Unmarshal(otBytes, &ot)
	if err != nil {
		return ot, err
	}

	return ot, nil
}

func (db *explorerDB) getAddressTransactions(address types.UnlockHash) ([]crypto.Hash, error) {
	var atxids []crypto.Hash
	txBytes, err := db.GetFromBucket("Addresses", encoding.Marshal(address))
	if err != nil {
		return atxids, err
	}

	err = encoding.Unmarshal(txBytes, &atxids)
	if err != nil {
		return atxids, err
	}

	return atxids, nil
}
