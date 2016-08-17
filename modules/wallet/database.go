package wallet

import (
	"reflect"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	// bucketHistoricClaimStarts maps a SiafundOutputID to the value of the
	// siafund pool when the output was processed. It stores every such output
	// in the blockchain. The wallet uses this mapping to determine the "claim
	// start" value of siafund outputs in ProcessedTransactions.
	bucketHistoricClaimStarts = []byte("bucketHistoricClaimStarts")
	// bucketHistoricOutputs maps a generic OutputID to the number of siacoins
	// the output contains. The output may be a siacoin or siafund output.
	// Note that the siafund value here is not the same as the value in
	// bucketHistoricClaimStarts; see the definition of SiafundOutput in
	// types/transactions.go for an explanation. The wallet uses this mapping
	// to determine the value of outputs in ProcessedTransactions.
	bucketHistoricOutputs = []byte("bucketHistoricOutputs")
	// bucketProcessedTransactions maps a TransactionID to a
	// ProcessedTransaction. Only transactions relevant to the wallet are
	// stored.
	bucketProcessedTransactions = []byte("bucketProcessedTransactions")
	// bucketSiacoinOutputs maps a SiacoinOutputID to its SiacoinOutput. Only
	// outputs that the wallet controls are stored. The wallet uses these
	// outputs to fund transactions.
	bucketSiacoinOutputs = []byte("bucketSiacoinOutputs")
	// bucketSiacoinOutputs maps a SiafundOutputID to its SiafundOutput. Only
	// outputs that the wallet controls are stored. The wallet uses these
	// outputs to fund transactions.
	bucketSiafundOutputs = []byte("bucketSiafundOutputs")
	// bucketSpentOutputs maps an OutputID to the height at which it was
	// spent. Only outputs spent by the wallet are stored. The wallet tracks
	// these outputs so that it can reuse them if they are not confirmed on
	// the blockchain.
	bucketSpentOutputs = []byte("bucketSpentOutputs")

	dbBuckets = [][]byte{
		bucketHistoricClaimStarts,
		bucketHistoricOutputs,
		bucketProcessedTransactions,
		bucketSiacoinOutputs,
		bucketSiafundOutputs,
		bucketSpentOutputs,
	}
)

// dbPut is a helper function for storing a marshalled key/value pair.
func dbPut(b *bolt.Bucket, key, val interface{}) error {
	return b.Put(encoding.Marshal(key), encoding.Marshal(val))
}

// dbGet is a helper function for retrieving a marshalled key/value pair. val
// must be a pointer.
func dbGet(b *bolt.Bucket, key, val interface{}) error {
	return encoding.Unmarshal(b.Get(encoding.Marshal(key)), val)
}

// dbDelete is a helper function for deleting a marshalled key/value pair.
func dbDelete(b *bolt.Bucket, key interface{}) error {
	return b.Delete(encoding.Marshal(key))
}

// dbForEach is a helper function for iterating over a bucket and calling fn
// on each entry. fn must be a function with two parameters. The key/value
// bytes of each bucket entry will be unmarshalled into the types of fn's
// parameters.
func dbForEach(b *bolt.Bucket, fn interface{}) error {
	// check function type
	fnVal, fnTyp := reflect.ValueOf(fn), reflect.TypeOf(fn)
	if fnTyp.Kind() != reflect.Func || fnTyp.NumIn() != 2 {
		panic("bad fn type: needed func(key, val), got " + fnTyp.String())
	}

	return b.ForEach(func(keyBytes, valBytes []byte) error {
		key, val := reflect.New(fnTyp.In(0)), reflect.New(fnTyp.In(1))
		if err := encoding.Unmarshal(keyBytes, key.Interface()); err != nil {
			return err
		} else if err := encoding.Unmarshal(valBytes, val.Interface()); err != nil {
			return err
		}
		fnVal.Call([]reflect.Value{key.Elem(), val.Elem()})
		return nil
	})
}

// Type-safe wrappers around the db helpers

func dbPutHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID, c types.Currency) error {
	return dbPut(tx.Bucket(bucketHistoricClaimStarts), id, c)
}
func dbGetHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID) (c types.Currency, err error) {
	err = dbGet(tx.Bucket(bucketHistoricClaimStarts), id, &c)
	return
}

func dbPutHistoricOutput(tx *bolt.Tx, id types.OutputID, c types.Currency) error {
	return dbPut(tx.Bucket(bucketHistoricOutputs), id, c)
}
func dbGetHistoricOutput(tx *bolt.Tx, id types.OutputID) (c types.Currency, err error) {
	err = dbGet(tx.Bucket(bucketHistoricOutputs), id, &c)
	return
}

func dbPutProcessedTransaction(tx *bolt.Tx, id types.TransactionID, pt modules.ProcessedTransaction) error {
	return dbPut(tx.Bucket(bucketProcessedTransactions), id, pt)
}
func dbGetProcessedTransaction(tx *bolt.Tx, id types.TransactionID) (pt modules.ProcessedTransaction, err error) {
	err = dbGet(tx.Bucket(bucketProcessedTransactions), id, &pt)
	return
}
func dbDeleteProcessedTransaction(tx *bolt.Tx, id types.TransactionID) error {
	return dbDelete(tx.Bucket(bucketProcessedTransactions), id)
}
func dbForEachProcessedTransaction(tx *bolt.Tx, fn func(types.TransactionID, modules.ProcessedTransaction)) error {
	return dbForEach(tx.Bucket(bucketProcessedTransactions), fn)
}

func dbPutSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID, output types.SiacoinOutput) error {
	return dbPut(tx.Bucket(bucketSiacoinOutputs), id, output)
}
func dbGetSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) (output types.SiacoinOutput, err error) {
	err = dbGet(tx.Bucket(bucketSiacoinOutputs), id, &output)
	return
}
func dbDeleteSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) error {
	return dbDelete(tx.Bucket(bucketSiacoinOutputs), id)
}
func dbForEachSiacoinOutput(tx *bolt.Tx, fn func(types.SiacoinOutputID, types.SiacoinOutput)) error {
	return dbForEach(tx.Bucket(bucketSiacoinOutputs), fn)
}

func dbPutSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID, output types.SiafundOutput) error {
	return dbPut(tx.Bucket(bucketSiafundOutputs), id, output)
}
func dbGetSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) (output types.SiafundOutput, err error) {
	err = dbGet(tx.Bucket(bucketSiafundOutputs), id, &output)
	return
}
func dbDeleteSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) error {
	return dbDelete(tx.Bucket(bucketSiafundOutputs), id)
}
func dbForEachSiafundOutput(tx *bolt.Tx, fn func(types.SiafundOutputID, types.SiafundOutput)) error {
	return dbForEach(tx.Bucket(bucketSiafundOutputs), fn)
}

func dbPutSpentOutput(tx *bolt.Tx, id types.OutputID, height types.BlockHeight) error {
	return dbPut(tx.Bucket(bucketSpentOutputs), id, height)
}
func dbGetSpentOutput(tx *bolt.Tx, id types.OutputID) (height types.BlockHeight, err error) {
	err = dbGet(tx.Bucket(bucketSpentOutputs), id, &height)
	return
}
func dbDeleteSpentOutput(tx *bolt.Tx, id types.OutputID) error {
	return dbDelete(tx.Bucket(bucketSpentOutputs), id)
}
