package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
)

// HttpGET is a utility function for making http get requests to sia with a whitelisted user-agent
func HttpGET(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", "Sia-Agent")
	return new(http.Client).Do(req)
}

// HttpPOST is a utility function for making post requests to sia with a whitelisted user-agent
func HttpPOST(url string, data string) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", "Sia-Agent")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return new(http.Client).Do(req)
}

// requireUserAgent is middleware that requires all requests to set a
// UserAgent that contains the specified string.
func requireUserAgent(h http.Handler, ua string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.UserAgent(), ua) {
			writeError(w, "Browser access disabled due to security vulnerability. Use Sia-UI or siac.", http.StatusBadRequest)
			return
		}
		h.ServeHTTP(w, req)
	})
}

// initAPI determines which functions handle each API call.
func (srv *Server) initAPI() {
	mux := httprouter.New()
	mux.NotFound = http.HandlerFunc(srv.unrecognizedCallHandler) // custom 404

	// Daemon API Calls - Unfinished
	mux.HandlerFunc("GET", "/daemon/constants", srv.daemonConstantsHandler)
	mux.HandlerFunc("GET", "/daemon/version", srv.daemonVersionHandler)
	mux.HandlerFunc("GET", "/daemon/stop", srv.daemonStopHandler)
	mux.HandlerFunc("GET", "/daemon/updates/apply", srv.daemonUpdatesApplyHandler)
	mux.HandlerFunc("GET", "/daemon/updates/check", srv.daemonUpdatesCheckHandler)

	// Consensus API Calls
	if srv.cs != nil {
		mux.GET("/consensus", srv.consensusHandler)
		mux.GET("/consensus/blocks/:height", srv.consensusBlocksHandler)
	}

	// Explorer API Calls
	if srv.explorer != nil {
		mux.GET("/explorer", srv.explorerHandler)
		mux.GET("/explorer/hash/:hash", srv.explorerHashHandler)
		mux.GET("/explorer/blocks/:height", srv.explorerBlocksHandler)
	}

	// Gateway API Calls - Unfinished
	if srv.gateway != nil {
		mux.GET("/gateway/status", srv.gatewayStatusHandler)
		mux.POST("/gateway/peers/add/:addr", srv.gatewayPeersAddHandler)
		mux.POST("/gateway/peers/remove/:addr", srv.gatewayPeersRemoveHandler)
	}

	// Host API Calls
	if srv.host != nil {
		mux.GET("/host", srv.hostHandlerGET)
		mux.POST("/host", srv.hostHandlerPOST)
		mux.POST("/host/announce", srv.hostAnnounceHandler)
	}

	// HostDB API Calls - DEPRECATED
	if srv.renter != nil {
		mux.HandlerFunc("GET", "/hostdb/hosts/active", srv.renterHostsActiveHandler)
		mux.HandlerFunc("GET", "/hostdb/hosts/all", srv.renterHostsAllHandler)
	}

	// Miner API Calls
	if srv.miner != nil {
		mux.GET("/miner", srv.minerHandler)
		mux.GET("/miner/header", srv.minerHeaderHandlerGET)
		mux.POST("/miner/header", srv.minerHeaderHandlerPOST)
		mux.POST("/miner/start", srv.minerStartHandler)
		mux.POST("/miner/stop", srv.minerStopHandler)
		mux.GET("/miner/headerforwork", srv.minerHeaderHandlerGET) // COMPATv0.4.8
		mux.GET("/miner/submitheader", srv.minerHeaderHandlerPOST) // COMPATv0.4.8
	}

	// Renter API Calls - Unfinished
	if srv.renter != nil {
		mux.HandlerFunc("GET", "/renter/downloadqueue", srv.renterDownloadqueueHandler)
		mux.HandlerFunc("GET", "/renter/files/delete", srv.renterFilesDeleteHandler)
		mux.HandlerFunc("GET", "/renter/files/download", srv.renterFilesDownloadHandler)
		mux.HandlerFunc("GET", "/renter/files/list", srv.renterFilesListHandler)
		mux.HandlerFunc("GET", "/renter/files/load", srv.renterFilesLoadHandler)
		mux.HandlerFunc("GET", "/renter/files/loadascii", srv.renterFilesLoadAsciiHandler)
		mux.HandlerFunc("GET", "/renter/files/rename", srv.renterFilesRenameHandler)
		mux.HandlerFunc("GET", "/renter/files/share", srv.renterFilesShareHandler)
		mux.HandlerFunc("GET", "/renter/files/shareascii", srv.renterFilesShareAsciiHandler)
		mux.HandlerFunc("GET", "/renter/files/upload", srv.renterFilesUploadHandler)
		mux.HandlerFunc("GET", "/renter/status", srv.renterStatusHandler)
	}

	// TransactionPool API Calls - Unfinished
	if srv.tpool != nil {
		mux.GET("/transactionpool/transactions", srv.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if srv.wallet != nil {
		mux.HandlerFunc("GET", "/wallet", srv.walletHandler)
		mux.HandlerFunc("GET", "/wallet/address", srv.walletAddressHandler)
		mux.HandlerFunc("GET", "/wallet/addresses", srv.walletAddressesHandler)
		mux.HandlerFunc("GET", "/wallet/backup", srv.walletBackupHandler)
		mux.HandlerFunc("POST", "/wallet/encrypt", srv.walletEncryptHandler) // COMPATv0.4.0
		mux.HandlerFunc("POST", "/wallet/init", srv.walletInitHandler)
		mux.HandlerFunc("POST", "/wallet/load/033x", srv.walletLoad033xHandler)
		mux.HandlerFunc("POST", "/wallet/load/seed", srv.walletLoadSeedHandler)
		mux.HandlerFunc("POST", "/wallet/load/siag", srv.walletLoadSiagHandler)
		mux.HandlerFunc("POST", "/wallet/lock", srv.walletLockHandler)
		mux.HandlerFunc("GET", "/wallet/seeds", srv.walletSeedsHandler)
		mux.HandlerFunc("POST", "/wallet/siacoins", srv.walletSiacoinsHandler)
		mux.HandlerFunc("POST", "/wallet/siafunds", srv.walletSiafundsHandler)
		mux.HandlerFunc("GET", "/wallet/transaction/:id", srv.walletTransactionHandler)
		mux.HandlerFunc("GET", "/wallet/transactions", srv.walletTransactionsHandler)
		mux.HandlerFunc("GET", "/wallet/transactions/:addr", srv.walletTransactionsHandler)
		mux.HandlerFunc("POST", "/wallet/unlock", srv.walletUnlockHandler)
	}

	// Apply UserAgent middleware and create HTTP server
	uaMux := requireUserAgent(mux, srv.requiredUserAgent)
	srv.apiServer = &http.Server{Handler: uaMux}
}

// unrecognizedCallHandler handles calls to unknown pages (404).
func (srv *Server) unrecognizedCallHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "404 - Refer to API.md")
}

// writeError an error to the API caller.
func writeError(w http.ResponseWriter, msg string, err int) {
	http.Error(w, msg, err)
}

// writeJSON writes the object to the ResponseWriter. If the encoding fails, an
// error is written instead.
func writeJSON(w http.ResponseWriter, obj interface{}) {
	if json.NewEncoder(w).Encode(obj) != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// writeSuccess writes the success json object ({"Success":true}) to the
// ResponseWriter
func writeSuccess(w http.ResponseWriter) {
	writeJSON(w, struct{ Success bool }{true})
}
