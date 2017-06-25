package transactionpool

import (
	"encoding/json"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// database.go contains objects related to the layout of the transaction pool's
// database, as well as getters and setters. Logic for interacting with the
// database can be found in persist.go

// Buckets in the database.
var (
	// bucketBlockHeight holds the most recent block height seen by the
	// transaction pool.
	bucketBlockHeight = []byte("BlockHeight")

	// bucketConfirmedTransactions holds the ids of every transaction that has
	// been confirmed on the blockchain.
	bucketConfirmedTransactions = []byte("ConfirmedTransactions")

	// bucketFeeMedian stores all of the persist data relating to the fee
	// median.
	bucketFeeMedian = []byte("FeeMedian")

	// bucketRecentConsensusChange holds the most recent consensus change seen
	// by the transaction pool.
	bucketRecentConsensusChange = []byte("RecentConsensusChange")
)

// Explicitly named fields in the database.
var (
	// fieldRecentConsensusChange is the field in bucketRecentConsensusChange
	// that holds the value of the most recent consensus change.
	fieldRecentConsensusChange = []byte("RecentConsensusChange")

	// fieldBlockHeight is the field in bucketBlockHeight that holds the value of
	// the most recent block height.
	fieldBlockHeight = []byte("BlockHeight")

	// fieldFeeMedian is the fee median persist data stored in a fee median
	// field.
	fieldFeeMedian = []byte("FeeMedian")
)

// Complex objects that get stored in database fields.
type (
	// medianPersist is the json object that gets stored in the database so that
	// the transaction pool can persist its block based fee estimations.
	medianPersist struct {
		RecentMedians   []types.Currency
		RecentMedianFee types.Currency
	}
)

// deleteTransaction deletes a transaction from the list of confirmed
// transactions.
func (tp *TransactionPool) deleteTransaction(tx *bolt.Tx, id types.TransactionID) error {
	return tx.Bucket(bucketConfirmedTransactions).Delete(id[:])
}

// getBlockHeight returns the most recent block height from the database.
func (tp *TransactionPool) getBlockHeight(tx *bolt.Tx) (bh types.BlockHeight, err error) {
	err = encoding.Unmarshal(tx.Bucket(bucketBlockHeight).Get(fieldBlockHeight), &bh)
	return
}

// getFeeMedian will get the fee median struct stored in the database.
func (tp *TransactionPool) getFeeMedian(tx *bolt.Tx) (medianPersist, error) {
	medianBytes := tp.dbTx.Bucket(bucketFeeMedian).Get(fieldFeeMedian)
	if medianBytes == nil {
		return medianPersist{}, errNilFeeMedian
	}

	var mp medianPersist
	err := json.Unmarshal(medianBytes, &mp)
	if err != nil {
		return medianPersist{}, build.ExtendErr("unable to unmarshal median data:", err)
	}
	return mp, nil
}

// getRecentConsensusChange returns the most recent consensus change from the
// database.
func (tp *TransactionPool) getRecentConsensusChange(tx *bolt.Tx) (cc modules.ConsensusChangeID, err error) {
	ccBytes := tx.Bucket(bucketRecentConsensusChange).Get(fieldRecentConsensusChange)
	if ccBytes == nil {
		return modules.ConsensusChangeID{}, errNilConsensusChange
	}
	copy(cc[:], ccBytes)
	return cc, nil
}

// putBlockHeight updates the transaction pool's block height.
func (tp *TransactionPool) putBlockHeight(tx *bolt.Tx, height types.BlockHeight) error {
	tp.blockHeight = height
	return tx.Bucket(bucketBlockHeight).Put(fieldBlockHeight, encoding.Marshal(height))
}

// putFeeMedian puts a median fees object into the database.
func (tp *TransactionPool) putFeeMedian(tx *bolt.Tx, mp medianPersist) error {
	objBytes, err := json.Marshal(mp)
	if err != nil {
		return err
	}
	return tx.Bucket(bucketFeeMedian).Put(fieldFeeMedian, objBytes)
}

// putRecentConsensusChange updates the most recent consensus change seen by
// the transaction pool.
func (tp *TransactionPool) putRecentConsensusChange(tx *bolt.Tx, cc modules.ConsensusChangeID) error {
	return tx.Bucket(bucketRecentConsensusChange).Put(fieldRecentConsensusChange, cc[:])
}

// putTransaction adds a transaction to the list of confirmed transactions.
func (tp *TransactionPool) putTransaction(tx *bolt.Tx, id types.TransactionID) error {
	return tx.Bucket(bucketConfirmedTransactions).Put(id[:], []byte{})
}
