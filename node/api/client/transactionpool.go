package client

import (
	"encoding/base64"
	"net/url"

	"github.com/NebulousLabs/Sia/encoding"
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
