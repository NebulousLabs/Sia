package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// WalletAddressGet requests a new address from the /wallet/address endpoint
func (c *Client) WalletAddressGet() (wag api.WalletAddressGET, err error) {
	err = c.get("/wallet/address", &wag)
	return
}

// WalletInitPost uses the /wallet/init endpoint to initialize and encrypt a
// wallet
func (c *Client) WalletInitPost(password string, force bool) (wip api.WalletInitPOST, err error) {
	values := url.Values{}
	values.Set("encryptionpassword", password)
	values.Set("force", strconv.FormatBool(force))
	err = c.post("/wallet/init", values.Encode(), &wip)
	return
}

// WalletGet requests the /wallet api resource
func (c *Client) WalletGet() (wg api.WalletGET, err error) {
	err = c.get("/wallet", &wg)
	return
}

// WalletSiacoinsMultiPost uses the /wallet/siacoin api endpoint to send money
// to multiple addresses at once
func (c *Client) WalletSiacoinsMultiPost(outputs []types.SiacoinOutput) (wsp api.WalletSiacoinsPOST, err error) {
	values := url.Values{}
	marshaledOutputs, err := json.Marshal(outputs)
	if err != nil {
		return api.WalletSiacoinsPOST{}, err
	}
	values.Set("outputs", string(marshaledOutputs))
	err = c.post("/wallet/siacoins", values.Encode(), &wsp)
	return
}

// WalletSiacoinsPost uses the /wallet/siacoins api endpoint to send money to a
// single address
func (c *Client) WalletSiacoinsPost(amount types.Currency, destination types.UnlockHash) (wsp api.WalletSiacoinsPOST, err error) {
	values := url.Values{}
	values.Set("amount", amount.String())
	values.Set("destination", destination.String())
	err = c.post("/wallet/siacoins", values.Encode(), &wsp)
	return
}

// WalletSignPost uses the /wallet/sign api endpoint to sign a transaction.
func (c *Client) WalletSignPost(txn types.Transaction, toSign []types.OutputID) (wspr api.WalletSignPOSTResp, err error) {
	buf := new(bytes.Buffer)
	err = json.NewEncoder(buf).Encode(api.WalletSignPOSTParams{
		Transaction: txn,
		ToSign:      toSign,
	})
	if err != nil {
		return
	}
	err = c.post("/wallet/sign", string(buf.Bytes()), &wspr)
	return
}

// WalletTransactionsGet requests the/wallet/transactions api resource for a
// certain startheight and endheight
func (c *Client) WalletTransactionsGet(startHeight types.BlockHeight, endHeight types.BlockHeight) (wtg api.WalletTransactionsGET, err error) {
	err = c.get(fmt.Sprintf("/wallet/transactions?startheight=%v&endheight=%v",
		startHeight, endHeight), &wtg)
	return
}

// WalletUnlockPost uses the /wallet/unlock endpoint to unlock the wallet with
// a given encryption key. Per default this key is the seed.
func (c *Client) WalletUnlockPost(password string) (err error) {
	values := url.Values{}
	values.Set("encryptionpassword", password)
	err = c.post("/wallet/unlock", values.Encode(), nil)
	return
}

// WalletUnspentGet requests the /wallet/unspent endpoint and returns all of
// the unspent outputs related to the wallet.
func (c *Client) WalletUnspentGet() (wug api.WalletUnspentGET, err error) {
	err = c.get("/wallet/unspent", &wug)
	return
}
