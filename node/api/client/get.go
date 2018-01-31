package client

import (
	"fmt"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// ConsensusGet requests the /consensus api resource
func (c *Client) ConsensusGet() (cg api.ConsensusGET, err error) {
	err = c.Get("/consensus", &cg)
	return
}

// GatewayGet requests the /gateway api resource
func (c *Client) GatewayGet() (gwg api.GatewayGET, err error) {
	err = c.Get("/gateway", &gwg)
	return
}

// MinerHeaderGet uses the /miner/header endpoint to get a header for work.
func (c *Client) MinerHeaderGet() (target types.Target, bh types.BlockHeader, err error) {
	targetAndHeader, err := c.GetRawResponse("/miner/header")
	if err != nil {
		return types.Target{}, types.BlockHeader{}, err
	}
	err = encoding.UnmarshalAll(targetAndHeader, &target, &bh)
	return
}

// WalletAddressGet requests a new address from the /wallet/address endpoint
func (c *Client) WalletAddressGet() (wag api.WalletAddressGET, err error) {
	err = c.Get("/wallet/address", &wag)
	return
}

// WalletGet requests the /wallet api resource
func (c *Client) WalletGet() (wg api.WalletGET, err error) {
	err = c.Get("/wallet", &wg)
	return
}

// WalletTransactionsGet requests the/wallet/transactions api resource for a
// certain startheight and endheight
func (c *Client) WalletTransactionsGet(startHeight types.BlockHeight, endHeight types.BlockHeight) (wtg api.WalletTransactionsGET, err error) {
	err = c.Get(fmt.Sprintf("/wallet/transactions?startheight=%v&endheight=%v",
		startHeight, endHeight), &wtg)
	return
}
