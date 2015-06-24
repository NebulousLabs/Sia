package blockexplorer

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
	"github.com/boltdb/bolt"
)

/*
Database Structure:

Blocks:
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

var meta persist.Metadata = persist.Metadata{
	Version: ".1",
	Header:  "Sia Block Explorer Database",
}

type explorerDB struct {
	*persist.BoltDatabase
}

type blockData struct {
	Block  types.Block
	Height types.BlockHeight
}

// Stored along with the height of a block. These are designed to be
// values that are easily accessable in sequence, so that the entire
// block does not need to be loaded
type blockSummary struct {
	ID        types.BlockID
	Timestamp types.Timestamp
	Target    types.Target
	Size      uint64
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
	hashFileRevision
	hashFileProof
	hashCoinOutputID
	hashFundOutputID
)

// OpenDB creates and opens the database for the block explorer. Sholud be run on startup
func openDB(filename string) (*explorerDB, error) {
	db, err := persist.OpenDatabase(meta, filename)
	if err != nil {
		return nil, err
	}

	var buckets []string = []string{
		"Blocks", "Transactions", "Addresses",
		"FileContracts", "StorageProofs", "FileProofs",
		"SiacoinOutputs", "SiafundOutputs", "Heights",
		"Hashes",
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

// Returns the block with a given id
func (db *explorerDB) getBlock(id types.BlockID) (block types.Block, err error) {
	var b []byte
	err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("Blocks"))
		if build.DEBUG {
			if bucket == nil {
				panic("blocks bucket was not created correcty")
			}
		}

		b = bucket.Get(encoding.Marshal(id))
		if b == nil {
			return errors.New("requested block does not exist in the database")
		}

		return nil
	})
	if err != nil {
		return block, err
	}
	err = encoding.Unmarshal(b, block)
	if err != nil {
		return block, err
	}

	return block, nil
}

// Returns an array of block summaries. Bounds checking should be done elsewhere
func (db *explorerDB) dbBlockSummaries(start types.BlockHeight, finish types.BlockHeight) ([]modules.ExplorerBlockData, error) {
	summaries := make([]modules.ExplorerBlockData, int(finish-start))
	err := db.View(func(tx *bolt.Tx) error {
		fmt.Printf("Beginning lookup\n")
		heights := tx.Bucket([]byte("Heights"))

		// Iterate over each block height, constructing a
		// summary data structure for each block
		for i := start; i < finish; i++ {
			bSummaryBytes := heights.Get(encoding.Marshal(types.BlockHeight(i)))
			if bSummaryBytes == nil {
				return errors.New("Block not found in height bucket")
			}

			var bSummary blockSummary
			err := encoding.Unmarshal(bSummaryBytes, &bSummary)
			if err != nil {
				return err
			}

			summaries[i-start] = modules.ExplorerBlockData{
				Timestamp: bSummary.Timestamp,
				Target:    bSummary.Target,
				Size:      bSummary.Size,
			}
		}
		fmt.Printf("Lookup finished\n")
		return nil
	})
	if err != nil {
		return nil, err
	}
	return summaries, nil
}
