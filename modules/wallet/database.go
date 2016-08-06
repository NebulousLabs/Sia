package wallet

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
)

var (
	bucketHistoricClaimStarts = []byte("bucketHistoricClaimStarts")
	bucketHistoricOutputs     = []byte("bucketHistoricOutputs")
	bucketSiacoinOutputs      = []byte("bucketSiacoinOutputs")
	bucketSiafundOutputs      = []byte("bucketSiafundOutputs")
	bucketSpentOutputs        = []byte("bucketSpentOutputs")

	dbBuckets = [][]byte{
		bucketHistoricClaimStarts,
		bucketHistoricOutputs,
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

// dbPutHistoricClaimStart stores the number of siafunds corresponding to the
// specified SiafundOutputID. The wallet builds a mapping of output -> value
// so that it can determine the value of siafund outputs that were not created
// by the wallet.
func dbPutHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID, c types.Currency) error {
	return dbPut(tx.Bucket(bucketHistoricClaimStarts), id, c)
}

// dbGetHistoricClaimStart retrieves the number of siafunds corresponding to
// the specified SiafundOutputID. The wallet uses this mapping to determine
// the value of siafund outputs that were not created by the wallet.
func dbGetHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID) (c types.Currency, err error) {
	err = dbGet(tx.Bucket(bucketHistoricClaimStarts), id, &c)
	return
}

// dbPutHistoricOutput stores the number of coins corresponding to the
// specified OutputID. The wallet builds a mapping of output -> value so that
// it can determine the value of outputs that were not created
// by the wallet.
func dbPutHistoricOutput(tx *bolt.Tx, id types.OutputID, c types.Currency) error {
	return dbPut(tx.Bucket(bucketHistoricOutputs), id, c)
}

// dbGetHistoricOutput retrieves the number of coins corresponding to the
// specified OutputID. The wallet uses this mapping to determine the value of
// outputs that were not created by the wallet.
func dbGetHistoricOutput(tx *bolt.Tx, id types.OutputID) (c types.Currency, err error) {
	err = dbGet(tx.Bucket(bucketHistoricOutputs), id, &c)
	return
}

// dbPutSiacoinOutput stores the output corresponding to the specified
// SiacoinOutputID.
func dbPutSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID, output types.SiacoinOutput) error {
	return dbPut(tx.Bucket(bucketSiacoinOutputs), id, output)
}

// dbGetSiacoinOutput retrieves the output corresponding to the specified
// SiacoinOutputID.
func dbGetSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) (output types.SiacoinOutput, err error) {
	err = dbGet(tx.Bucket(bucketSiacoinOutputs), id, &output)
	return
}

// dbDeleteSiacoinOutput deletes the output corresponding to the specified
// SiacoinOutputID.
func dbDeleteSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) error {
	return dbDelete(tx.Bucket(bucketSiacoinOutputs), id)
}

// dbPutSiafundOutput stores the output corresponding to the specified
// SiafundOutputID.
func dbPutSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID, output types.SiafundOutput) error {
	return dbPut(tx.Bucket(bucketSiafundOutputs), id, output)
}

// dbGetSiafundOutput retrieves the output corresponding to the specified
// SiafundOutputID.
func dbGetSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) (output types.SiafundOutput, err error) {
	err = dbGet(tx.Bucket(bucketSiafundOutputs), id, &output)
	return
}

// dbDeleteSiafundOutput deletes the output corresponding to the specified
// SiafundOutputID.
func dbDeleteSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) error {
	return dbDelete(tx.Bucket(bucketSiafundOutputs), id)
}

// dbPutSpentOutput registers an output as being spent as of the specified
// height.
func dbPutSpentOutput(tx *bolt.Tx, id types.OutputID, height types.BlockHeight) error {
	return dbPut(tx.Bucket(bucketSpentOutputs), id, height)
}

// dbGetSpentOutput retrieves the height at which the specified output was
// spent.
func dbGetSpentOutput(tx *bolt.Tx, id types.OutputID) (height types.BlockHeight, err error) {
	err = dbGet(tx.Bucket(bucketSpentOutputs), id, &height)
	return
}

// dbDeleteSpentOutput deletes the output corresponding to the specified
// OutputID.
func dbDeleteSpentOutput(tx *bolt.Tx, id types.OutputID) error {
	return dbDelete(tx.Bucket(bucketSpentOutputs), id)
}
