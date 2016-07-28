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

func dbPutHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID, c types.Currency) error {
	return tx.Bucket(bucketHistoricClaimStarts).Put(encoding.Marshal(id), encoding.Marshal(c))
}

func dbGetHistoricClaimStart(tx *bolt.Tx, id types.SiafundOutputID) (c types.Currency, err error) {
	b := tx.Bucket(bucketHistoricClaimStarts).Get(encoding.Marshal(id))
	err = encoding.Unmarshal(b, &c)
	return
}

func dbPutHistoricOutput(tx *bolt.Tx, id types.OutputID, c types.Currency) error {
	return tx.Bucket(bucketHistoricOutputs).Put(encoding.Marshal(id), encoding.Marshal(c))
}

func dbGetHistoricOutput(tx *bolt.Tx, id types.OutputID) (c types.Currency, err error) {
	b := tx.Bucket(bucketHistoricOutputs).Get(encoding.Marshal(id))
	err = encoding.Unmarshal(b, &c)
	return
}
