package main

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

// walletAddressHandler manages requests for CoinAddresses from the wallet.
func (d *daemon) walletAddressHandler(w http.ResponseWriter, req *http.Request) {
	coinAddress, _, err := d.wallet.CoinAddress()
	if err != nil {
		writeError(w, "Failed to get a coin address", 500)
		return
	}

	// Since coinAddress is not a struct, we define one here so that writeJSON
	// writes an object instead of a bare value. In addition, we transmit the
	// coinAddress as a hex-encoded string rather than a byte array.
	writeJSON(w, struct {
		Address string
	}{fmt.Sprintf("%x", coinAddress)})
}

// walletSendHandler manages 'send' requests that are made to the wallet.
func (d *daemon) walletSendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the inputs.
	var amount consensus.Currency
	var dest consensus.UnlockHash
	err := amount.UnmarshalJSON([]byte(req.FormValue("amount")))
	if err != nil {
		writeError(w, "Malformed amount", 400)
		return
	}

	// Parse the string into an address.
	destString := req.FormValue("dest")
	var destAddressBytes []byte
	_, err = fmt.Sscanf(destString, "%x", &destAddressBytes)
	if err != nil {
		writeError(w, "Malformed coin address", 400)
		return
	}
	copy(dest[:], destAddressBytes)

	// Spend the coins.
	_, err = d.wallet.SpendCoins(amount, dest)
	if err != nil {
		writeError(w, "Failed to create transaction: "+err.Error(), 500)
		return
	}

	writeSuccess(w)
}

// walletStatusHandler returns a struct containing wallet information, like the
// balance.
func (d *daemon) walletStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.wallet.Info())
}
