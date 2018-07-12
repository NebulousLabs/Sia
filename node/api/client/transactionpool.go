package client

import (
	"encoding/base64"
	"net/url"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// TransactionpoolRawPost uses the /tpool/raw endpoint to broadcast a
// transaction by adding it to the transactionpool.
func (c *Client) TransactionpoolRawPost(parents []types.Transaction, txn types.Transaction) (err error) {
	parentsBytes := encoding.Marshal(parents)
	txnBytes := encoding.Marshal(txn)
	values := url.Values{}
	values.Set("parents", base64.StdEncoding.EncodeToString(parentsBytes))
	values.Set("transaction", base64.StdEncoding.EncodeToString(txnBytes))
	err = c.post("/tpool/raw", values.Encode(), nil)
	return
}

// TransactionPoolFeeGet uses the /tpool/fee endpoint to get a fee estimation.
func (c *Client) TransactionPoolFeeGet() (tfg api.TpoolFeeGET, err error) {
	err = c.get("/tpool/fee", &tfg)
	return
}

// TransactionPoolRawPost uses the /tpool/raw endpoint to send a raw
// transaction to the transaction pool.
func (c *Client) TransactionPoolRawPost(txn types.Transaction, parents []types.Transaction) (err error) {
	values := url.Values{}
	values.Set("transaction", string(encoding.Marshal(txn)))
	values.Set("parents", string(encoding.Marshal(parents)))
	err = c.post("/tpool/raw", values.Encode(), nil)
	return
}
