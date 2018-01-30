package client

import (
	"net/url"
	"strconv"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// PostWalletInit uses the /wallet/init endpoint to initialize and encrypt a
// wallet
func (c *Client) PostWalletInit(password string, force bool) (wip api.WalletInitPOST, err error) {
	values := url.Values{}
	values.Set("encryptionpassword", password)
	values.Set("force", strconv.FormatBool(force))
	err = c.Post("/wallet/init", values.Encode(), &wip)
	return
}

// PostWalletUnlock uses the /wallet/unlock endpoint to unlock the wallet with
// a given encryption key. Per default this key is the seed.
func (c *Client) PostWalletUnlock(password string) (err error) {
	values := url.Values{}
	values.Set("encryptionpassword", password)
	err = c.Post("/wallet/unlock", values.Encode(), nil)
	return
}

// PostMinerHeader uses the /miner/header endpoint to submit a solved block
// header that was previously received from the same endpoint
func (c *Client) PostMinerHeader(bh types.BlockHeader) (err error) {
	err = c.Post("/miner/header", string(encoding.Marshal(bh)), nil)
	return
}
