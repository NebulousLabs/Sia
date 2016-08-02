package wallet

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
)

var (
	bucketHistoricClaimStarts = []byte("bucketHistoricClaimStarts")
	bucketHistoricOutputs     = []byte("bucketHistoricOutputs")
)

// dbPutHistoricClaimStart stores the number of siafunds corresponding to the
// specified SiafundOutputID. The wallet builds a mapping of output -> value
// so that it can determine the value of siafund outputs that were not created
// by the wallet.
func dbPutHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID, c types.Currency) error {
	return tx.Bucket(bucketHistoricClaimStarts).Put(encoding.Marshal(id), encoding.Marshal(c))
}

// dbGetHistoricClaimStart retrieves the number of siafunds corresponding to
// the specified SiafundOutputID. The wallet uses this mapping to determine
// the value of siafund outputs that were not created by the wallet.
func dbGetHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID) (c types.Currency, err error) {
	b := tx.Bucket(bucketHistoricClaimStarts).Get(encoding.Marshal(id))
	err = encoding.Unmarshal(b, &c)
	return
}

// dbPutHistoricOutput stores the number of coins corresponding to the
// specified OutputID. The wallet builds a mapping of output -> value so that
// it can determine the value of outputs that were not created
// by the wallet.
func dbPutHistoricOutput(tx *bolt.Tx, id types.OutputID, c types.Currency) error {
	return tx.Bucket(bucketHistoricOutputs).Put(encoding.Marshal(id), encoding.Marshal(c))
}

// dbGetHistoricOutput retrieves the number of coins corresponding to the
// specified OutputID. The wallet uses this mapping to determine the value of
// outputs that were not created by the wallet.
func dbGetHistoricOutput(tx *bolt.Tx, id types.OutputID) (c types.Currency, err error) {
	b := tx.Bucket(bucketHistoricOutputs).Get(encoding.Marshal(id))
	err = encoding.Unmarshal(b, &c)
	return
}
