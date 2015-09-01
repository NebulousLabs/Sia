package api

import (
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/NebulousLabs/entropy-mnemonics"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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

	// WalletInitPOST contains the primary seed that gets generated during a
	// POST call to /wallet/init.
	WalletInitPOST struct {
		PrimarySeed string `json:"primaryseed"`
	}

	// WalletEncryptPOST contains the primary seed that gets generated during a
	// POST call to /wallet/encrypt.
	//
	// COMPATv0.4.0
	WalletEncryptPOST struct {
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

	// WalletSeedGet contains the seeds used by the wallet.
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

// scanAmount scans a types.Currency from a string.
func scanAmount(amount string) (types.Currency, bool) {
	// use SetString manually to ensure that amount does not contain
	// multiple values, which would confuse fmt.Scan
	i, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return types.Currency{}, ok
	}
	return types.NewCurrency(i), true
}

// scanAddres scans a types.UnlockHash from a string.
func scanAddress(addrStr string) (addr types.UnlockHash, err error) {
	err = addr.LoadString(addrStr)
	return
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
		srv.walletHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet", http.StatusBadRequest)
}

// walletAddressHandlerGET handles a GET request to /wallet/seed.
func (srv *Server) walletAddressHandlerGET(w http.ResponseWriter, req *http.Request) {
	unlockConditions, err := srv.wallet.NextAddress()
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletAddressGET{
		Address: unlockConditions.UnlockHash(),
	})
}

// walletAddressHandler handles API calls to /wallet/address.
func (srv *Server) walletAddressHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.walletAddressHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/address", http.StatusBadRequest)
}

// walletBackupHandlerPOST handles a POST call to /wallet/backup
func (srv *Server) walletBackupHandlerPOST(w http.ResponseWriter, req *http.Request) {
	err := srv.wallet.CreateBackup(req.FormValue("filepath"))
	if err != nil {
		writeError(w, "error after call to /wallet/backup: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// walletBackupHandler handles API calls to /wallet/backup.
func (srv *Server) walletBackupHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletBackupHandlerPOST(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/backup", http.StatusBadRequest)
}

// walletInitHandlerPOST handles a POST call to /wallet/init.
func (srv *Server) walletInitHandlerPOST(w http.ResponseWriter, req *http.Request) {
	var encryptionKey crypto.TwofishKey
	if req.FormValue("encryptionpassword") != "" {
		encryptionKey = crypto.TwofishKey(crypto.HashObject(req.FormValue("encryptionpassword")))
	}
	seed, err := srv.wallet.Encrypt(encryptionKey)
	if err != nil {
		writeError(w, "error when calling /wallet/init: "+err.Error(), http.StatusBadRequest)
		return
	}

	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictID == "" {
		dictID = "english"
	}
	seedStr, err := modules.SeedToString(seed, dictID)
	if err != nil {
		writeError(w, "error when calling /wallet/init: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletInitPOST{
		PrimarySeed: seedStr,
	})
}

// walletInitHandler handles API calls to /wallet/init.
func (srv *Server) walletInitHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletInitHandlerPOST(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/init", http.StatusBadRequest)
}

// walletEncryptHandler is a legacy alias for walletInitHandler.
//
// COMPATv0.4.0
func (srv *Server) walletEncryptHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletInitHandlerPOST(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/encrypt", http.StatusBadRequest)
}

// walletLoad033xHandlerPOST handles a POST call to /wallet/load/033x.
func (srv *Server) walletLoad033xHandlerPOST(w http.ResponseWriter, req *http.Request) {
	filepath := req.FormValue("filepath")
	potentialKeys := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := srv.wallet.Load033xWallet(key, filepath)
		if err == nil {
			writeSuccess(w)
			return
		}
		if err != nil && err != modules.ErrBadEncryptionKey {
			writeError(w, "error when calling /wallet/load/033x: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	writeError(w, modules.ErrBadEncryptionKey.Error(), http.StatusBadRequest)
}

// walletLoad033xHandler handles API calls to /wallet/load/033x.
func (srv *Server) walletLoad033xHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletLoad033xHandlerPOST(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/load/033x", http.StatusBadRequest)
}

// walletLockHandlerPOST handles a POST request to /wallet/lock.
func (srv *Server) walletLockHandlerPOST(w http.ResponseWriter, req *http.Request) {
	err := srv.wallet.Lock()
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// walletLockHanlder handles API calls to /wallet/lock.
func (srv *Server) walletLockHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletLockHandlerPOST(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/lock", http.StatusBadRequest)
}

// walletSeedsHandlerGET handles a GET request to /wallet/seeds.
func (srv *Server) walletSeedsHandlerGET(w http.ResponseWriter, req *http.Request) {
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
	allSeeds, err := srv.wallet.AllSeeds()
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}
	var allSeedsStrs []string
	for _, seed := range allSeeds {
		str, err := modules.SeedToString(seed, dictionary)
		if err != nil {
			writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
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

// walletSeedsHandlerPOST handles a POST request to /wallet/seeds.
func (srv *Server) walletSeedsHandlerPOST(w http.ResponseWriter, req *http.Request) {
	// Get the seed using the ditionary + phrase
	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	seed, err := modules.StringToSeed(req.FormValue("seed"), dictID)
	if err != nil {
		writeError(w, "error when calling /wallet/seeds: "+err.Error(), http.StatusBadRequest)
		return
	}

	potentialKeys := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := srv.wallet.RecoverSeed(key, seed)
		if err == nil {
			writeSuccess(w)
			return
		}
		if err != nil && err != modules.ErrBadEncryptionKey {
			writeError(w, "error when calling /wallet/seeds: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	writeError(w, "error when calling /wallet/seeds: "+modules.ErrBadEncryptionKey.Error(), http.StatusBadRequest)
}

// walletSeedHandler handles API calls to /wallet/seed.
func (srv *Server) walletSeedsHandler(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET", "":
		srv.walletSeedsHandlerGET(w, req)
	case "POST":
		srv.walletSeedsHandlerPOST(w, req)
	default:
		writeError(w, "unrecognized method when calling /wallet/seed", http.StatusBadRequest)
	}
}

// walletSiacoinsHandlerPOST handles a POST request to /wallet/siacoins.
func (srv *Server) walletSiacoinsHandlerPOST(w http.ResponseWriter, req *http.Request) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		writeError(w, "could not read 'amount' from POST call to /wallet/siacoins", http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		writeError(w, "error after call to /wallet/siacoins: "+err.Error(), http.StatusBadRequest)
		return
	}

	txns, err := srv.wallet.SendSiacoins(amount, dest)
	if err != nil {
		writeError(w, "error after call to /wallet/siacoins: "+err.Error(), http.StatusInternalServerError)
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

// walletSiacoinsHandler handles API calls to /wallet/siacoins.
func (srv *Server) walletSiacoinsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletSiacoinsHandlerPOST(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/siacoins", http.StatusBadRequest)
}

// walletSiafundsHandlerPOST handles a POST request to /wallet/siafunds.
func (srv *Server) walletSiafundsHandlerPOST(w http.ResponseWriter, req *http.Request) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		writeError(w, "could not read 'amount' from POST call to /wallet/siafunds", http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		writeError(w, "error after call to /wallet/siafunds: "+err.Error(), http.StatusBadRequest)
		return
	}

	txns, err := srv.wallet.SendSiafunds(amount, dest)
	if err != nil {
		writeError(w, "error after call to /wallet/siafunds: "+err.Error(), http.StatusInternalServerError)
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

// walletSiafundsHandler handles API calls to /wallet/siafunds.
func (srv *Server) walletSiafundsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletSiafundsHandlerPOST(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet/siafunds", http.StatusBadRequest)
	}
}

// walletTransactionHandlerGETid handles a GET call to
// /wallet/transaction/$(id).
func (srv *Server) walletTransactionHandlerGETid(w http.ResponseWriter, req *http.Request, id types.TransactionID) {
	txn, ok := srv.wallet.Transaction(id)
	if !ok {
		writeError(w, "error when calling /wallet/transaction/$(id): transaction not found", http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletTransactionGETid{
		Transaction: txn,
	})
}

// walletTransactionHandler handles API calls to /wallet/transaction.
func (srv *Server) walletTransactionHandler(w http.ResponseWriter, req *http.Request) {
	// GET is the only supported method.
	if req.Method != "" && req.Method != "GET" {
		writeError(w, "unrecognized method when calling /wallet/transaction", http.StatusBadRequest)
		return
	}

	// Parse the id from the url.
	var id types.TransactionID
	jsonID := "\"" + strings.TrimPrefix(req.URL.Path, "/wallet/transaction/") + "\""
	err := id.UnmarshalJSON([]byte(jsonID))
	if err != nil {
		writeError(w, "error after call to /wallet/history: "+err.Error(), http.StatusBadRequest)
		return
	}
	srv.walletTransactionHandlerGETid(w, req, id)
}

// walletTransactionsHandlerGET handles a GET call to /wallet/transactions.
func (srv *Server) walletTransactionsHandlerGET(w http.ResponseWriter, req *http.Request) {
	// Get the start and end blocks.
	start, err := strconv.Atoi(req.FormValue("startheight"))
	if err != nil {
		writeError(w, "error after call to /wallet/transactions: "+err.Error(), http.StatusBadRequest)
		return
	}
	end, err := strconv.Atoi(req.FormValue("endheight"))
	if err != nil {
		writeError(w, "error after call to /wallet/transactions: "+err.Error(), http.StatusBadRequest)
		return
	}
	confirmedTxns, err := srv.wallet.Transactions(types.BlockHeight(start), types.BlockHeight(end))
	if err != nil {
		writeError(w, "error after call to /wallet/transactions: "+err.Error(), http.StatusBadRequest)
		return
	}
	unconfirmedTxns := srv.wallet.UnconfirmedTransactions()

	writeJSON(w, WalletTransactionsGET{
		ConfirmedTransactions:   confirmedTxns,
		UnconfirmedTransactions: unconfirmedTxns,
	})
}

// walletTransactionsHandlerGETaddr handles a GET request to
// /wallet/transactions/$(addr).
func (srv *Server) walletTransactionsHandlerGETaddr(w http.ResponseWriter, req *http.Request, addr types.UnlockHash) {
	confirmedATs := srv.wallet.AddressTransactions(addr)
	unconfirmedATs := srv.wallet.AddressUnconfirmedTransactions(addr)
	writeJSON(w, WalletTransactionsGETaddr{
		ConfirmedTransactions:   confirmedATs,
		UnconfirmedTransactions: unconfirmedATs,
	})
}

// walletTransactionsHandler handles API calls to /wallet/transactions.
func (srv *Server) walletTransactionsHandler(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/wallet/transactions" && (req.Method == "" || req.Method == "GET") {
		srv.walletTransactionsHandlerGET(w, req)
		return
	}

	// Only a GET call is allowed at this point.
	if req.Method != "GET" && req.Method != "" {
		writeError(w, "unrecognized method call to /wallet/transactions", http.StatusBadRequest)
		return
	}

	// The only call remaining is /wallet/transactions/$(addr) - parse the
	// address.
	jsonAddr := "\"" + strings.TrimPrefix(req.URL.Path, "/wallet/transactions/") + "\""
	var addr types.UnlockHash
	err := addr.UnmarshalJSON([]byte(jsonAddr))
	if err != nil {
		writeError(w, "error after call to /wallet/transactions: "+err.Error(), http.StatusBadRequest)
		return
	}
	srv.walletTransactionsHandlerGETaddr(w, req, addr)
}

// walletUnlockHandlerPOST handles a POST call to /wallet/unlock.
func (srv *Server) walletUnlockHandlerPOST(w http.ResponseWriter, req *http.Request) {
	potentialKeys := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := srv.wallet.Unlock(key)
		if err == nil {
			writeSuccess(w)
			return
		}
		if err != nil && err != modules.ErrBadEncryptionKey {
			writeError(w, "error when calling /wallet/unlock: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	writeError(w, "error when calling /wallet/unlock: "+modules.ErrBadEncryptionKey.Error(), http.StatusBadRequest)
}

// walletUnlockHandler handles API calls to /wallet/unlock.
func (srv *Server) walletUnlockHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.walletUnlockHandlerPOST(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /wallet/unlock", http.StatusBadRequest)
}
