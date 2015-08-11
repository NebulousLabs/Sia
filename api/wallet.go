package api

import (
	"net/http"
	"strconv"

	"github.com/NebulousLabs/entropy-mnemonics"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// WalletGET contains general information about the wallet, with tags to
// support idiomatic json encodings.
type WalletGET struct {
	Encrypted bool `json:"encrypted"`
	Unlocked  bool `json:"unlocked"`

	ConfirmedSiacoinBalance     types.Currency `json:"confirmedSiacoinBalance"`
	UnconfirmedOutgoingSiacoins types.Currency `json:"unconfirmedOutgoingSiacoins"`
	UnconfirmedIncomingSiacoins types.Currency `json:"unconfirmedIncomingSiacoins"`

	SiafundBalance      types.Currency `json:"siafundBalance"`
	SiacoinClaimBalance types.Currency `json:"siacoinClaimBalance"`
}

// WalletHistoryGet contains wallet transaction history.
type WalletHistoryGET struct {
	UnconfirmedTransactions []modules.WalletTransaction `json:"unconfirmedTransactions"`
	ConfirmedTransactions   []modules.WalletTransaction `json:"confirmedTransactions"`
}

// WalletSeedGet contains the seeds used by the wallet.
type WalletSeedGET struct {
	PrimarySeed        string   `json:"primarySeed"`
	AddressesRemaining int      `json:"AddressesRemaining"`
	AllSeeds           []string `json:"allSeeds"`
}

// walletHandlerGET handles a GET request to /wallet.
func (srv *Server) walletHandlerGET(w http.ResponseWriter, req *http.Request) {
	siacoinBal, siafundBal, siaclaimBal := srv.wallet.ConfirmedBalance()
	siacoinsOut, siacoinsIn := srv.wallet.UnconfirmedBalance()
	writeJSON(w, WalletGET{
		Encrypted: srv.wallet.Encrypted(),
		Unlocked:  srv.wallet.Unlocked(),

		ConfirmedSiacoinBalance:     siacoinBal,
		UnconfirmedOutgoingSiacoins: siacoinsOut,
		UnconfirmedIncomingSiacoins: siacoinsIn,

		SiafundBalance:      siafundBal,
		SiacoinClaimBalance: siaclaimBal,
	})
}

// walletHander handles API calls to /wallet.
func (srv *Server) walletHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.consensusHandlerGET(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet", http.StatusBadRequest)
	}
}

// walletCloseHandlerPUT handles a PUT request to /wallet/close.
func (srv *Server) walletCloseHandlerPUT(w http.ResponseWriter, req *http.Request) {
	err := srv.wallet.Close()
	if err == nil {
		writeSuccess(w)
	} else {
		writeError(w, err.Error(), http.StatusBadRequest)
	}
}

// walletCloseHanlder handles API calls to /wallet/close.
func (srv *Server) walletCloseHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "PUT" {
		srv.walletCloseHandlerPUT(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet/close", http.StatusBadRequest)
	}
}

// walletHistoryHandlerGET handles a GET request to /wallet/history.
func (srv *Server) walletHistoryHandlerGET(w http.ResponseWriter, req *http.Request) {
	start, err := strconv.Atoi(req.FormValue("start"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
	}
	end, err := strconv.Atoi(req.FormValue("end"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
	}

	confirmedHistory, err := srv.wallet.TransactionHistory(types.BlockHeight(start), types.BlockHeight(end))
	if err != nil {
		writeError(w, "/walet/history [GET] Error:"+err.Error(), http.StatusBadRequest)
	}
	writeJSON(w, WalletHistoryGET{
		UnconfirmedTransactions: srv.wallet.UnconfirmedTransactions(),
		ConfirmedTransactions:   confirmedHistory,
	})
}

// walletHistoryHandlerGETAddr handles a GET request to
// /wallet/history/$(addr).
func (srv *Server) walletHistoryHandlerGETAddr(w http.ResponseWriter, req *http.Request, addr types.UnlockHash) {
	addrHistory, err := srv.wallet.AddressTransactionHistory(addr)
	if err != nil {
		writeError(w, "error after call to /wallet/history/$(addr): "+err.Error(), http.StatusBadRequest)
	}
	writeJSON(w, addrHistory)
}

// walletHistoryHandler handles all API calls to /wallet/history
func (srv *Server) walletHistoryHandler(w http.ResponseWriter, req *http.Request) {
	// Check for a vanilla call to /wallet/history.
	if req.URL.Path == "/wallet/history" && req.Method == "GET" || req.Method == "" {
		srv.walletHistoryHandlerGET(w, req)
	}

	// The only remaining possibility is a GET call to /wallet/history/$(addr);
	// check that the method is correct.
	if req.Method != "GET" && req.Method != "" {
		writeError(w, "unrecognized method in call to /wallet/history", http.StatusBadRequest)
	}

	// Parse the address from the url and call the GETAddr Handler.
	jsonAddr := "\"" + req.URL.Path[len("/wallet/history"):] + "\""
	var addr types.UnlockHash
	err := addr.UnmarshalJSON([]byte(jsonAddr))
	if err != nil {
		writeError(w, "error after call to /wallet/history: "+err.Error(), http.StatusBadRequest)
	}
	srv.walletHistoryHandlerGETAddr(w, req, addr)
}

// walletSeedHandlerGET handles a GET request to /wallet/seed.
func (srv *Server) walletSeedHandlerGET(w http.ResponseWriter, req *http.Request) {
	dictionary := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictionary == "" {
		dictionary = mnemonics.English
	}

	// Get the primary seed information.
	primarySeed, progress, err := srv.wallet.PrimarySeed()
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}
	primarySeedStr, err := modules.SeedToString(primarySeed, dictionary)
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get the list of seeds known to the wallet.
	allSeeds := srv.wallet.AllSeeds()
	var allSeedsStrs []string
	for _, seed := range allSeeds {
		str, err := modules.SeedToString(seed, dictionary)
		if err != nil {
			writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
			return
		}
		allSeedsStrs = append(allSeedsStrs, str)
	}

	writeJSON(w, WalletSeedGET{
		PrimarySeed:        primarySeedStr,
		AddressesRemaining: int(modules.PublicKeysPerSeed - progress),
		AllSeeds:           allSeedsStrs,
	})
}
