package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/entropy-mnemonics"
	"github.com/julienschmidt/httprouter"
)

type (
	// WalletGET contains general information about the wallet.
	WalletGET struct {
		Encrypted bool `json:"encrypted"`
		Unlocked  bool `json:"unlocked"`

		ConfirmedSiacoinBalance     types.Currency `json:"confirmedsiacoinbalance"`
		UnconfirmedOutgoingSiacoins types.Currency `json:"unconfirmedoutgoingsiacoins"`
		UnconfirmedIncomingSiacoins types.Currency `json:"unconfirmedincomingsiacoins"`

		SiafundBalance      types.Currency `json:"siafundbalance"`
		SiacoinClaimBalance types.Currency `json:"siacoinclaimbalance"`
	}

	// WalletAddressGET contains an address returned by a GET call to
	// /wallet/address.
	WalletAddressGET struct {
		Address types.UnlockHash `json:"address"`
	}

	// WalletAddressesGET contains the list of wallet addresses returned by a
	// GET call to /wallet/addresses.
	WalletAddressesGET struct {
		Addresses []types.UnlockHash `json:"addresses"`
	}

	// WalletInitPOST contains the primary seed that gets generated during a
	// POST call to /wallet/init.
	WalletInitPOST struct {
		PrimarySeed string `json:"primaryseed"`
	}

	// WalletSiacoinsPOST contains the transaction sent in the POST call to
	// /wallet/siafunds.
	WalletSiacoinsPOST struct {
		TransactionIDs []types.TransactionID `json:"transactionids"`
	}

	// WalletSiafundsPOST contains the transaction sent in the POST call to
	// /wallet/siafunds.
	WalletSiafundsPOST struct {
		TransactionIDs []types.TransactionID `json:"transactionids"`
	}

	// WalletSeedsGET contains the seeds used by the wallet.
	WalletSeedsGET struct {
		PrimarySeed        string   `json:"primaryseed"`
		AddressesRemaining int      `json:"addressesremaining"`
		AllSeeds           []string `json:"allseeds"`
	}

	// WalletTransactionGETid contains the transaction returned by a call to
	// /wallet/transaction/$(id)
	WalletTransactionGETid struct {
		Transaction modules.ProcessedTransaction `json:"transaction"`
	}

	// WalletTransactionsGET contains the specified set of confirmed and
	// unconfirmed transactions.
	WalletTransactionsGET struct {
		ConfirmedTransactions   []modules.ProcessedTransaction `json:"confirmedtransactions"`
		UnconfirmedTransactions []modules.ProcessedTransaction `json:"unconfirmedtransactions"`
	}

	// WalletTransactionsGETaddr contains the set of wallet transactions
	// relevant to the input address provided in the call to
	// /wallet/transaction/$(addr)
	WalletTransactionsGETaddr struct {
		ConfirmedTransactions   []modules.ProcessedTransaction `json:"confirmedtransactions"`
		UnconfirmedTransactions []modules.ProcessedTransaction `json:"unconfirmedtransactions"`
	}
)

// encryptionKeys enumerates the possible encryption keys that can be derived
// from an input string.
func encryptionKeys(seedStr string) (validKeys []crypto.TwofishKey) {
	dicts := []mnemonics.DictionaryID{"english", "german", "japanese"}
	for _, dict := range dicts {
		seed, err := modules.StringToSeed(seedStr, dict)
		if err != nil {
			continue
		}
		validKeys = append(validKeys, crypto.TwofishKey(crypto.HashObject(seed)))
	}
	validKeys = append(validKeys, crypto.TwofishKey(crypto.HashObject(seedStr)))
	return validKeys
}

// walletHander handles API calls to /wallet.
func (srv *Server) walletHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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

// wallet033xHandler handles API calls to /wallet/033x.
func (srv *Server) wallet033xHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	source := req.FormValue("source")
	potentialKeys := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := srv.wallet.Load033xWallet(key, source)
		if err == nil {
			writeSuccess(w)
			return
		}
		if err != nil && err != modules.ErrBadEncryptionKey {
			writeError(w, APIError{"error when calling /wallet/033x: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	writeError(w, APIError{modules.ErrBadEncryptionKey.Error()}, http.StatusBadRequest)
}

// walletAddressHandler handles API calls to /wallet/address.
func (srv *Server) walletAddressHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	unlockConditions, err := srv.wallet.NextAddress()
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/addresses: " + err.Error()}, http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletAddressGET{
		Address: unlockConditions.UnlockHash(),
	})
}

// walletAddressHandler handles API calls to /wallet/addresses.
func (srv *Server) walletAddressesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, WalletAddressesGET{
		Addresses: srv.wallet.AllAddresses(),
	})
}

// walletBackupHandler handles API calls to /wallet/backup.
func (srv *Server) walletBackupHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := srv.wallet.CreateBackup(req.FormValue("destination"))
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/backup: " + err.Error()}, http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// walletInitHandler handles API calls to /wallet/init.
func (srv *Server) walletInitHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var encryptionKey crypto.TwofishKey
	if req.FormValue("encryptionpassword") != "" {
		encryptionKey = crypto.TwofishKey(crypto.HashObject(req.FormValue("encryptionpassword")))
	}
	seed, err := srv.wallet.Encrypt(encryptionKey)
	if err != nil {
		writeError(w, APIError{"error when calling /wallet/init: " + err.Error()}, http.StatusBadRequest)
		return
	}

	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictID == "" {
		dictID = "english"
	}
	seedStr, err := modules.SeedToString(seed, dictID)
	if err != nil {
		writeError(w, APIError{"error when calling /wallet/init: " + err.Error()}, http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletInitPOST{
		PrimarySeed: seedStr,
	})
}

// walletSeedHandler handles API calls to /wallet/seed.
func (srv *Server) walletSeedHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Get the seed using the ditionary + phrase
	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	seed, err := modules.StringToSeed(req.FormValue("seed"), dictID)
	if err != nil {
		writeError(w, APIError{"error when calling /wallet/seed: " + err.Error()}, http.StatusBadRequest)
		return
	}

	potentialKeys := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := srv.wallet.LoadSeed(key, seed)
		if err == nil {
			writeSuccess(w)
			return
		}
		if err != nil && err != modules.ErrBadEncryptionKey {
			writeError(w, APIError{"error when calling /wallet/seed: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	writeError(w, APIError{"error when calling /wallet/seed: " + modules.ErrBadEncryptionKey.Error()}, http.StatusBadRequest)
}

// walletSiagkeyHandler handles API calls to /wallet/siagkey.
func (srv *Server) walletSiagkeyHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Fetch the list of keyfiles from the post body.
	keyfiles := strings.Split(req.FormValue("keyfiles"), ",")
	potentialKeys := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := srv.wallet.LoadSiagKeys(key, keyfiles)
		if err == nil {
			writeSuccess(w)
			return
		}
		if err != nil && err != modules.ErrBadEncryptionKey {
			writeError(w, APIError{"error when calling /wallet/siagkey: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	writeError(w, APIError{"error when calling /wallet/siagkey: " + modules.ErrBadEncryptionKey.Error()}, http.StatusBadRequest)
}

// walletLockHanlder handles API calls to /wallet/lock.
func (srv *Server) walletLockHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := srv.wallet.Lock()
	if err != nil {
		writeError(w, APIError{err.Error()}, http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// walletSeedsHandler handles API calls to /wallet/seeds.
func (srv *Server) walletSeedsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	dictionary := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictionary == "" {
		dictionary = mnemonics.English
	}

	// Get the primary seed information.
	primarySeed, progress, err := srv.wallet.PrimarySeed()
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
		return
	}
	primarySeedStr, err := modules.SeedToString(primarySeed, dictionary)
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
		return
	}

	// Get the list of seeds known to the wallet.
	allSeeds, err := srv.wallet.AllSeeds()
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
		return
	}
	var allSeedsStrs []string
	for _, seed := range allSeeds {
		str, err := modules.SeedToString(seed, dictionary)
		if err != nil {
			writeError(w, APIError{"error after call to /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
			return
		}
		allSeedsStrs = append(allSeedsStrs, str)
	}
	writeJSON(w, WalletSeedsGET{
		PrimarySeed:        primarySeedStr,
		AddressesRemaining: int(modules.PublicKeysPerSeed - progress),
		AllSeeds:           allSeedsStrs,
	})
}

// walletSiacoinsHandler handles API calls to /wallet/siacoins.
func (srv *Server) walletSiacoinsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		writeError(w, APIError{"could not read 'amount' from POST call to /wallet/siacoins"}, http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/siacoins: " + err.Error()}, http.StatusBadRequest)
		return
	}

	txns, err := srv.wallet.SendSiacoins(amount, dest)
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/siacoins: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	var txids []types.TransactionID
	for _, txn := range txns {
		txids = append(txids, txn.ID())
	}
	writeJSON(w, WalletSiacoinsPOST{
		TransactionIDs: txids,
	})
}

// walletSiafundsHandler handles API calls to /wallet/siafunds.
func (srv *Server) walletSiafundsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		writeError(w, APIError{"could not read 'amount' from POST call to /wallet/siafunds"}, http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/siafunds: " + err.Error()}, http.StatusBadRequest)
		return
	}

	txns, err := srv.wallet.SendSiafunds(amount, dest)
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/siafunds: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	var txids []types.TransactionID
	for _, txn := range txns {
		txids = append(txids, txn.ID())
	}
	writeJSON(w, WalletSiafundsPOST{
		TransactionIDs: txids,
	})
}

// walletTransactionHandler handles API calls to /wallet/transaction/:id.
func (srv *Server) walletTransactionHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Parse the id from the url.
	var id types.TransactionID
	jsonID := "\"" + ps.ByName("id") + "\""
	err := id.UnmarshalJSON([]byte(jsonID))
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/history: " + err.Error()}, http.StatusBadRequest)
		return
	}

	txn, ok := srv.wallet.Transaction(id)
	if !ok {
		writeError(w, APIError{"error when calling /wallet/transaction/$(id): transaction not found"}, http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletTransactionGETid{
		Transaction: txn,
	})
}

// walletTransactionsHandler handles API calls to /wallet/transactions.
func (srv *Server) walletTransactionsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	startheightStr, endheightStr := req.FormValue("startheight"), req.FormValue("endheight")
	if startheightStr == "" || endheightStr == "" {
		writeError(w, APIError{"startheight and endheight must be provided to a /wallet/transactions call."}, http.StatusBadRequest)
		return
	}
	// Get the start and end blocks.
	start, err := strconv.Atoi(startheightStr)
	if err != nil {
		writeError(w, APIError{"parsing integer value for parameter `startheight` failed: " + err.Error()}, http.StatusBadRequest)
		return
	}
	end, err := strconv.Atoi(endheightStr)
	if err != nil {
		writeError(w, APIError{"parsing integer value for parameter `endheight` failed: " + err.Error()}, http.StatusBadRequest)
		return
	}
	confirmedTxns, err := srv.wallet.Transactions(types.BlockHeight(start), types.BlockHeight(end))
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/transactions: " + err.Error()}, http.StatusBadRequest)
		return
	}
	unconfirmedTxns := srv.wallet.UnconfirmedTransactions()

	writeJSON(w, WalletTransactionsGET{
		ConfirmedTransactions:   confirmedTxns,
		UnconfirmedTransactions: unconfirmedTxns,
	})
}

// walletTransactionsAddrHandler handles API calls to
// /wallet/transactions/:addr.
func (srv *Server) walletTransactionsAddrHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Parse the address being input.
	jsonAddr := "\"" + ps.ByName("addr") + "\""
	var addr types.UnlockHash
	err := addr.UnmarshalJSON([]byte(jsonAddr))
	if err != nil {
		writeError(w, APIError{"error after call to /wallet/transactions: " + err.Error()}, http.StatusBadRequest)
		return
	}

	confirmedATs := srv.wallet.AddressTransactions(addr)
	unconfirmedATs := srv.wallet.AddressUnconfirmedTransactions(addr)
	writeJSON(w, WalletTransactionsGETaddr{
		ConfirmedTransactions:   confirmedATs,
		UnconfirmedTransactions: unconfirmedATs,
	})
}

// walletUnlockHandler handles API calls to /wallet/unlock.
func (srv *Server) walletUnlockHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	potentialKeys := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := srv.wallet.Unlock(key)
		if err == nil {
			writeSuccess(w)
			return
		}
		if err != nil && err != modules.ErrBadEncryptionKey {
			writeError(w, APIError{"error when calling /wallet/unlock: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	writeError(w, APIError{"error when calling /wallet/unlock: " + modules.ErrBadEncryptionKey.Error()}, http.StatusBadRequest)
}
