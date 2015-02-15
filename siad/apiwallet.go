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

	// TODO: Explain what's happening here, and why it doesn't look like all of
	// the other calls.
	writeJSON(w, struct {
		Address string
	}{fmt.Sprintf("%x", coinAddress)})
}

// walletSendHandler manages 'send' requests that are made to the wallet.
func (d *daemon) walletSendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the inputs.
	var amount consensus.Currency
	var dest consensus.UnlockHash
	_, err := fmt.Sscan(req.FormValue("amount"), &amount)
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
	walletStatus, err := d.wallet.Info()
	if err != nil {
		writeError(w, "Failed to get wallet info", 500)
		return
	}
	writeJSON(w, walletStatus)
}
