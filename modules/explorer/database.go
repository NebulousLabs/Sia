package explorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
	"github.com/boltdb/bolt"
)

/*
Database Structure:

Blocks
  types.BlockID ----------> types.Block
Transactions
  Txid -------------------> txInfo
Addresses
  UnlockHash -------------> []types.Txid
FileContracts
  types.FcID -------------> fcInfo

SiacoinOutputs
  SiacoinOutputID --------> outputTransactions
SiafundOutputs
  SiafundOutputID --------> outputTransactions

Heights
  types.BlockHeight ------> types.BlockID
Hashes
  crypto.Hash ------------> hash type (int)

*/

var meta = persist.Metadata{
	Version: "0.1",
	Header:  "Sia Block Explorer Database",
}

type explorerDB struct {
	*persist.BoltDatabase
}

type blockData struct {
	Block  types.Block
	Height types.BlockHeight
}

// txInfo provides enough information to find the actual transaction
// in the block database
type txInfo struct {
	BlockID types.BlockID
	TxNum   int // Not possible to have >2^32 with block size
}

// fcInfo provides enough information to easily find the file
// contracts in the blockchain
type fcInfo struct {
	Contract  crypto.Hash
	Revisions []crypto.Hash
	Proof     crypto.Hash
}

// outputTransactions stores enough information to go from an output id to
// the places where it is used
type outputTransactions struct {
	OutputTx crypto.Hash
	InputTx  crypto.Hash
}

// Create an enum (a bunch of integers with iota) for the hash type lookup
const (
	hashBlock = iota
	hashTransaction
	hashFilecontract
	hashCoinOutputID
	hashFundOutputID
	hashUnlockHash
)

// OpenDB creates and opens the database for the block explorer. Sholud be run on startup
func openDB(filename string) (*explorerDB, error) {
	db, err := persist.OpenDatabase(meta, filename)
	if err != nil {
		return nil, err
	}

	var buckets []string = []string{
		"Blocks", "Transactions", "Addresses",
		"FileContracts", "SiacoinOutputs", "SiafundOutputs",
		"Heights", "Hashes",
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucketName := range buckets {
			_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &explorerDB{db}, nil
}

// Returns an array of block summaries. Bounds checking should be done elsewhere
func (db *explorerDB) dbBlockSummaries(start types.BlockHeight, finish types.BlockHeight) ([]modules.ExplorerBlockData, error) {
	summaries := make([]modules.ExplorerBlockData, int(finish-start))
	err := db.View(func(tx *bolt.Tx) error {
		heights := tx.Bucket([]byte("Heights"))

		// Iterate over each block height, constructing a
		// summary data structure for each block
		for i := start; i < finish; i++ {
			bSummaryBytes := heights.Get(encoding.Marshal(i))
			if bSummaryBytes == nil {
				return errors.New("Block not found in height bucket")
			}

			err := encoding.Unmarshal(bSummaryBytes, &summaries[i-start])
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return summaries, nil
}
