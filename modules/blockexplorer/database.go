package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
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

StorageProofs
  StorageProofID ---------> FileContrat
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
	Target types.Target
}

// txInfo provides enough information to find the actual transaction
// in the block database
type txInfo struct {
	blockID types.BlockID
	txNum   int // Not possible to have >2^32 with block size
}

// fcInfo provides enough information to easily find the file
// contracts in the blockchain
type fcInfo struct {
	contract  crypto.Hash
	revisions crypto.Hash
	proof     crypto.Hash
}

// outputTransactions stores enough information to go from an output id to
// the places where it is used
type outputTransactions struct {
	outputTx crypto.Hash
	inputTx  crypto.Hash
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

// A higher level function to insert a block into the database
func (db *explorerDB) insertBlock(b types.Block) error {
	err := db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("Blocks"))
		if build.DEBUG {
			if bucket == nil {
				panic("blocks bucket was not created correcty")
			}
		}

		err := bucket.Put(encoding.Marshal(b.ID()), encoding.Marshal(b))
		if err != nil {
			return err
		}
		return nil
	})
	return err
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
