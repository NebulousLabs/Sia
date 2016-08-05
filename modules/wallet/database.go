package wallet

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
)

var (
	bucketHistoricClaimStarts = []byte("bucketHistoricClaimStarts")
	bucketHistoricOutputs     = []byte("bucketHistoricOutputs")

	dbBuckets = [][]byte{
		bucketHistoricClaimStarts,
		bucketHistoricOutputs,
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
