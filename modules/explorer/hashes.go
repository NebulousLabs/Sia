package explorer

import (
	"bytes"
	"errors"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	responseBlock        = "Block"
	responseTransaction  = "Transaction"
	responseFileContract = "FileContract"
	responseOutput       = "Output"
	responseAddress      = "Address"
)

// getFromBucket returns an object fetched from a bucket.
//
// DEPRECATED
func (db *explorerDB) getFromBucket(bucketName string, key []byte) (obj []byte, err error) {
	dbErr := db.View(func(tx *bolt.Tx) error {
		obj = tx.Bucket([]byte(bucketName)).Get(key)
		return nil
	})
	if dbErr != nil {
		return nil, dbErr
	}
	return obj, nil
}

// GetHashInfo returns sufficient data about the hash that was
// provided to do more extensive lookups
func (e *Explorer) GetHashInfo(hash []byte) (interface{}, error) {
	if len(hash) < crypto.HashSize {
		return nil, errors.New("requested hash not long enough")
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// Perform a lookup to tell which type of hash it is
	typeBytes, err := e.db.getFromBucket("Hashes", hash[:crypto.HashSize])
	if err != nil {
		return nil, err
	}

	var hashType int
	err = encoding.Unmarshal(typeBytes, &hashType)
	if err != nil {
		return nil, err
	}

	switch hashType {
	case hashBlock:
		var id types.BlockID
		copy(id[:], hash[:])
		return e.db.getBlock(types.BlockID(id))
	case hashTransaction:
		var id crypto.Hash
		copy(id[:], hash[:])
		return e.db.getTransaction(id)
	case hashFilecontract:
		var id types.FileContractID
		copy(id[:], hash[:])
		return e.db.getFileContract(id)
	case hashCoinOutputID:
		var id types.SiacoinOutputID
		copy(id[:], hash[:])
		return e.db.getSiacoinOutput(id)
	case hashFundOutputID:
		var id types.SiafundOutputID
		copy(id[:], hash[:])
		return e.db.getSiafundOutput(id)
	case hashUnlockHash:
		// Check that the address is valid before doing a lookup
		if len(hash) != crypto.HashSize+types.UnlockHashChecksumSize {
			return nil, errors.New("address does not have a valid checksum")
		}
		var id types.UnlockHash
		copy(id[:], hash[:crypto.HashSize])
		uhChecksum := crypto.HashObject(id)

		givenChecksum := hash[crypto.HashSize : crypto.HashSize+types.UnlockHashChecksumSize]
		if !bytes.Equal(givenChecksum, uhChecksum[:types.UnlockHashChecksumSize]) {
			return nil, errors.New("address does not have a valid checksum")
		}

		return e.db.getAddressTransactions(id)
	default:
		return nil, errors.New("bad hash type")
	}
}

// Returns the block with a given id
func (db *explorerDB) getBlock(id types.BlockID) (modules.BlockResponse, error) {
	var br modules.BlockResponse

	b, err := db.getFromBucket("Blocks", encoding.Marshal(id))
	if err != nil {
		return br, err
	}

	var bd blockData
	err = encoding.Unmarshal(b, &bd)
	if err != nil {
		return br, err
	}
	br.Block = bd.Block
	br.Height = bd.Height
	br.ResponseType = responseBlock
	return br, nil
}

// Returns the transaction with the given id
func (db *explorerDB) getTransaction(id crypto.Hash) (modules.TransactionResponse, error) {
	var tr modules.TransactionResponse

	// Look up the transaction's location
	tBytes, err := db.getFromBucket("Transactions", encoding.Marshal(id))
	if err != nil {
		return tr, err
	}

	var tLocation txInfo
	err = encoding.Unmarshal(tBytes, &tLocation)
	if err != nil {
		return tr, err
	}

	// Look up the block specified by the location and extract the transaction
	bBytes, err := db.getFromBucket("Blocks", encoding.Marshal(tLocation.BlockID))
	if err != nil {
		return tr, err
	}

	var block types.Block
	err = encoding.Unmarshal(bBytes, &block)
	if err != nil {
		return tr, err
	}
	tr.Tx = block.Transactions[tLocation.TxNum]
	tr.ParentID = tLocation.BlockID
	tr.TxNum = tLocation.TxNum
	tr.ResponseType = responseTransaction
	return tr, nil
}

// Returns the list of transactions a file contract with a given id has taken part in
func (db *explorerDB) getFileContract(id types.FileContractID) (modules.FcResponse, error) {
	var fr modules.FcResponse
	fcBytes, err := db.getFromBucket("FileContracts", encoding.Marshal(id))
	if err != nil {
		return fr, err
	}

	var fc fcInfo
	err = encoding.Unmarshal(fcBytes, &fc)
	if err != nil {
		return fr, err
	}

	fr.Contract = fc.Contract
	fr.Revisions = fc.Revisions
	fr.Proof = fc.Proof
	fr.ResponseType = responseFileContract

	return fr, nil
}

// getSiacoinOutput retrieves data about a siacoin output from the
// database and puts it into a response structure
func (db *explorerDB) getSiacoinOutput(id types.SiacoinOutputID) (modules.OutputResponse, error) {
	var or modules.OutputResponse
	otBytes, err := db.getFromBucket("SiacoinOutputs", encoding.Marshal(id))
	if err != nil {
		return or, err
	}

	var ot outputTransactions
	err = encoding.Unmarshal(otBytes, &ot)
	if err != nil {
		return or, err
	}

	or.OutputTx = ot.OutputTx
	or.InputTx = ot.InputTx
	or.ResponseType = responseOutput

	return or, nil
}

// getSiafundOutput retrieves data about a siafund output and puts it
// into a response structure
func (db *explorerDB) getSiafundOutput(id types.SiafundOutputID) (modules.OutputResponse, error) {
	var or modules.OutputResponse
	otBytes, err := db.getFromBucket("SiafundOutputs", encoding.Marshal(id))
	if err != nil {
		return or, err
	}

	var ot outputTransactions
	err = encoding.Unmarshal(otBytes, &ot)
	if err != nil {
		return or, err
	}

	or.OutputTx = ot.OutputTx
	or.InputTx = ot.InputTx
	or.ResponseType = responseOutput

	return or, nil
}

// getAddressTransactions gets the list of all blocks and transactions
// the address was involved with, which could be many, and puts the
// result in a response structure
func (db *explorerDB) getAddressTransactions(address types.UnlockHash) (modules.AddrResponse, error) {
	var ar modules.AddrResponse
	txBytes, err := db.getFromBucket("Addresses", encoding.Marshal(address))
	if err != nil {
		return ar, err
	}

	var atxids []types.TransactionID
	err = encoding.Unmarshal(txBytes, &atxids)
	if err != nil {
		return ar, err
	}

	ar.Txns = atxids
	ar.ResponseType = responseAddress

	return ar, nil
}
