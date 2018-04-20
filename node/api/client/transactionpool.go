package client

import "github.com/NebulousLabs/Sia/node/api"

// TransactionPoolFeeGet uses the /tpool/fee endpoint to get a fee estimation.
func (c *Client) TransactionPoolFeeGet() (tfg api.TpoolFeeGET, err error) {
	err = c.get("/tpool/fee", &tfg)
	return
}
