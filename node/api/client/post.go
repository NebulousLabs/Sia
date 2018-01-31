package client

import (
	"net/url"
	"strconv"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// GatewayConnectPost uses the /gateway/connect/:address endpoint to connect to
// the gateway at address
func (c *Client) GatewayConnectPost(address modules.NetAddress) (err error) {
	err = c.Post("/gateway/connect/"+string(address), "", nil)
	return
}

// MinerHeaderPost uses the /miner/header endpoint to submit a solved block
// header that was previously received from the same endpoint
func (c *Client) MinerHeaderPost(bh types.BlockHeader) (err error) {
	err = c.Post("/miner/header", string(encoding.Marshal(bh)), nil)
	return
}

// WalletInitPost uses the /wallet/init endpoint to initialize and encrypt a
// wallet
func (c *Client) WalletInitPost(password string, force bool) (wip api.WalletInitPOST, err error) {
	values := url.Values{}
	values.Set("encryptionpassword", password)
	values.Set("force", strconv.FormatBool(force))
	err = c.Post("/wallet/init", values.Encode(), &wip)
	return
}

// WalletUnlockPost uses the /wallet/unlock endpoint to unlock the wallet with
// a given encryption key. Per default this key is the seed.
func (c *Client) WalletUnlockPost(password string) (err error) {
	values := url.Values{}
	values.Set("encryptionpassword", password)
	err = c.Post("/wallet/unlock", values.Encode(), nil)
	return
}
