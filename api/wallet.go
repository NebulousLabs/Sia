package api

import (
	"math/big"
	"net/http"
	// "strings"

	"github.com/NebulousLabs/Sia/types"
)

// WalletSiafundsBalance contains fields relating to the siafunds balance.
type WalletSiafundsBalance struct {
	SiafundBalance      types.Currency
	SiacoinClaimBalance types.Currency
}

// scanAmount scans a types.Currency.
func scanAmount(amount string) (types.Currency, bool) {
	// use SetString manually to ensure that amount does not contain
	// multiple values, which would confuse fmt.Scan
	i, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return types.Currency{}, ok
	}
	return types.NewCurrency(i), true
}

// scanAddres scans a types.UnlockHash.
func scanAddress(addrStr string) (addr types.UnlockHash, err error) {
	err = addr.LoadString(addrStr)
	return
}

// walletAddressHandler handles the API request for a new address.
func (srv *Server) walletAddressHandler(w http.ResponseWriter, req *http.Request) {
	unlockConditions, err := srv.wallet.NextAddress()
	if err != nil {
		writeError(w, "Failed to get a coin address", http.StatusInternalServerError)
		return
	}

	// Since coinAddress is not a struct, we define one here so that writeJSON
	// writes an object instead of a bare value. In addition, we transmit the
	// coinAddress as a hex-encoded string rather than a byte array.
	writeJSON(w, struct{ Address types.UnlockHash }{unlockConditions.UnlockHash()})
}

/*
// walletMergeHandler handles the API call to merge a different wallet into the
// current wallet.
func (srv *Server) walletMergeHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the wallet file.
	err := srv.wallet.MergeWallet(req.FormValue("walletfile"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}
*/

// walletSendHandler handles the API call to send coins to another address.
func (srv *Server) walletSendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the amount.
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		writeError(w, "Malformed amount", http.StatusBadRequest)
		return
	}

	// Scan the destination address.
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		writeError(w, "Malformed coin address", http.StatusBadRequest)
		return
	}

	// Send the coins.
	_, err = srv.wallet.SendSiacoins(amount, dest)
	if err != nil {
		writeError(w, "Failed to create transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// walletSiafundsBalanceHandler handles the API call querying the balance of
// siafunds.
func (srv *Server) walletSiafundsBalanceHandler(w http.ResponseWriter, req *http.Request) {
	var wsb WalletSiafundsBalance
	_, wsb.SiafundBalance, wsb.SiacoinClaimBalance = srv.wallet.ConfirmedBalance()
	writeJSON(w, wsb)
}

// walletSiafundsSendHandler handles the API request to send siafunds.
func (srv *Server) walletSiafundsSendHandler(w http.ResponseWriter, req *http.Request) {
	/*
		// Scan the amount.
		amount, ok := scanAmount(req.FormValue("amount"))
		if !ok {
			writeError(w, "Malformed amount", http.StatusBadRequest)
			return
		}

		// Scan the destination address.
		dest, err := scanAddress(req.FormValue("destination"))
		if err != nil {
			writeError(w, "Malformed coin address", http.StatusBadRequest)
			return
		}

		// Scan the keyfile list.
		keyfiles := strings.Split(req.FormValue("keyfiles"), ",")
		if len(keyfiles) == 0 {
			writeError(w, "Missing keyfiles", http.StatusBadRequest)
			return
		}

		// _, err = srv.wallet.SendSiagSiafunds(amount, dest, keyfiles)
		if err != nil {
			writeError(w, "Failed to send siafunds: "+err.Error(), http.StatusInternalServerError)
			return
		}
	*/

	// writeSuccess(w)
	writeError(w, "Wallet does not currently implement siafunds functions", http.StatusBadRequest)
}

// walletSiafundsWatchsiagaddressHandler handles the API request to watch a
// siag address.
func (srv *Server) walletSiafundsWatchsiagaddressHandler(w http.ResponseWriter, req *http.Request) {
	/*
		err := srv.wallet.WatchSiagSiafundAddress(req.FormValue("keyfile"))
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
	*/
	// writeSuccess(w)
	writeError(w, "Wallet does not currently implement siafunds functions", http.StatusBadRequest)
}

// walletStatusHandler handles the API call querying the status of the wallet.
func (srv *Server) walletStatusHandler(w http.ResponseWriter, req *http.Request) {
	// writeJSON(w, srv.wallet.Info())
	writeError(w, "Wallet status not currently implemented", http.StatusBadRequest)
}
