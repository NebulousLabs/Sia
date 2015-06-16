package api

import (
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/types"
)

// WalletSiafundsBalance contains fields relating to the siafunds balance.
type WalletSiafundsBalance struct {
	SiafundBalance      types.Currency
	SiacoinClaimBalance types.Currency
}

// scanAmount scans a types.Currency.
func scanAmount(amountStr string) (amount types.Currency, err error) {
	// exponential format
	if strings.ContainsAny(amountStr, "Ee") {
		amountRat := new(big.Rat)
		_, err = fmt.Sscan(amountStr, amountRat)
		if err != nil {
			return
		}
		amount = types.NewCurrency(new(big.Int).Div(amountRat.Num(), amountRat.Denom()))
		return
	}

	// standard format
	_, err = fmt.Sscan(amountStr, &amount)
	return
}

// scanAddres scans a types.UnlockHash.
func scanAddress(addrStr string) (addr types.UnlockHash, err error) {
	err = addr.LoadString(addrStr)
	return
}

// walletAddressHandler handles the API request for a new address.
func (srv *Server) walletAddressHandler(w http.ResponseWriter, req *http.Request) {
	coinAddress, _, err := srv.wallet.CoinAddress(true) // true indicates that the address should be visible to the user
	if err != nil {
		writeError(w, "Failed to get a coin address", http.StatusInternalServerError)
		return
	}

	// Since coinAddress is not a struct, we define one here so that writeJSON
	// writes an object instead of a bare value. In addition, we transmit the
	// coinAddress as a hex-encoded string rather than a byte array.
	writeJSON(w, struct{ Address types.UnlockHash }{coinAddress})
}

// walletSendHandler handles the API call to send coins to another address.
func (srv *Server) walletSendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the amount.
	amount, err := scanAmount(req.FormValue("amount"))
	if err != nil {
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
	_, err = srv.wallet.SendCoins(amount, dest)
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
	wsb.SiafundBalance, wsb.SiacoinClaimBalance = srv.wallet.SiafundBalance()
	writeJSON(w, wsb)
}

// walletSiafundsSendHandler handles the API request to send siafunds.
func (srv *Server) walletSiafundsSendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the amount.
	amount, err := scanAmount(req.FormValue("amount"))
	if err != nil {
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

	_, err = srv.wallet.SendSiagSiafunds(amount, dest, keyfiles)
	if err != nil {
		writeError(w, "Failed to send siafunds: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// walletSiafundsWatchsiagaddressHandler handles the API request to watch a
// siag address.
func (srv *Server) walletSiafundsWatchsiagaddressHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.wallet.WatchSiagSiafundAddress(req.FormValue("keyfile"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// walletStatusHandler handles the API call querying the status of the wallet.
func (srv *Server) walletStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.wallet.Info())
}
