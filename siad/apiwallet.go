package main

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

// walletSendHandler manages 'send' requests that are made to the wallet.
func (d *daemon) walletSendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the inputs.
	var amount consensus.Currency
	var dest consensus.CoinAddress
	_, err := fmt.Sscan(req.FormValue("amount"), &amount)
	if err != nil {
		http.Error(w, "Malformed amount", 400)
		return
	}

	// dest can be either a coin address or a friend name
	destString := req.FormValue("dest")
	// if ca, ok := e.friends[destString]; ok {
	//	dest = ca
	//} else
	if len(destString) != 64 {
		http.Error(w, "Friend not found (or malformed coin address)", 400)
		return
	} else {
		var destAddressBytes []byte
		_, err = fmt.Sscanf(destString, "%x", &destAddressBytes)
		if err != nil {
			http.Error(w, "Malformed coin address", 400)
			return
		}
		copy(dest[:], destAddressBytes)
	}

	// Spend the coins.
	_, err = d.core.SpendCoins(amount, dest)
	if err != nil {
		http.Error(w, "Failed to create transaction: "+err.Error(), 500)
		return
	}

	fmt.Fprintf(w, "Sent %v coins to %x", amount, dest)
}

// I wasn't sure the best way to manage this. I've implemented it so that the
// wallet returns some arbitrary JSON and it's up to the front-end to figure
// out how to use the json. The daemon and envrionment don't really know what's
// contained within in an attempt to keep things modular.
func (d *daemon) walletStatusHandler(w http.ResponseWriter, req *http.Request) {
	json, err := d.core.WalletInfo()
	if err != nil {
		http.Error(w, "Failed to encode status object", 500)
		return
	}
	w.Write(json)
}

// walletAddressHandler manages requests for CoinAddresses from the wallet.
func (d *daemon) walletAddressHandler(w http.ResponseWriter, req *http.Request) {
	coinAddress, err := d.core.CoinAddress()
	if err != nil {
		http.Error(w, "Failed to get a coin address", 500)
		return
	}
	fmt.Fprintf(w, "You can now use this address: %x", coinAddress)
}
