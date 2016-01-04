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
	mux.GET("/daemon/constants", srv.daemonConstantsHandler)
	mux.GET("/daemon/version", srv.daemonVersionHandler)
	mux.GET("/daemon/stop", srv.daemonStopHandler)

	// Consensus API Calls
	if srv.cs != nil {
		mux.GET("/consensus", srv.consensusHandler)
	}

	// Explorer API Calls
	if srv.explorer != nil {
		mux.GET("/explorer", srv.explorerHandler)
		mux.GET("/explorer/blocks/:height", srv.explorerBlocksHandler)
		mux.GET("/explorer/hashes/:hash", srv.explorerHashHandler)
	}

	// Gateway API Calls - Unfinished
	if srv.gateway != nil {
		mux.GET("/gateway", srv.gatewayHandler)
		mux.POST("/gateway/add/:addr", srv.gatewayAddHandler)
		mux.POST("/gateway/remove/:addr", srv.gatewayRemoveHandler)
	}

	// Host API Calls
	if srv.host != nil {
		mux.GET("/host", srv.hostHandlerGET)
		mux.POST("/host", srv.hostHandlerPOST)
		mux.POST("/host/announce", srv.hostAnnounceHandler)
	}

	// Miner API Calls
	if srv.miner != nil {
		mux.GET("/miner", srv.minerHandler)
		mux.GET("/miner/header", srv.minerHeaderHandlerGET)
		mux.POST("/miner/header", srv.minerHeaderHandlerPOST)
		mux.GET("/miner/start", srv.minerStartHandler)
		mux.GET("/miner/stop", srv.minerStopHandler)
		mux.GET("/miner/headerforwork", srv.minerHeaderHandlerGET) // COMPATv0.4.8
		mux.GET("/miner/submitheader", srv.minerHeaderHandlerPOST) // COMPATv0.4.8
	}

	// Renter API Calls - Unfinished
	if srv.renter != nil {
		mux.GET("/renter/downloads", srv.renterDownloadsHandler)
		mux.GET("/renter/files", srv.renterFilesHandler)

		mux.POST("/renter/load", srv.renterLoadHandler)
		mux.POST("/renter/loadascii", srv.renterLoadAsciiHandler)
		mux.GET("/renter/share", srv.renterShareHandler)
		mux.GET("/renter/shareascii", srv.renterShareAsciiHandler)

		mux.POST("/renter/delete/*path", srv.renterDeleteHandler)
		mux.GET("/renter/download/*path", srv.renterDownloadHandler)
		mux.POST("/renter/rename/*path", srv.renterRenameHandler)
		mux.POST("/renter/upload/*path", srv.renterUploadHandler)

		mux.GET("/renter/hosts/active", srv.renterHostsActiveHandler)
		mux.GET("/renter/hosts/all", srv.renterHostsAllHandler)
	}

	// TransactionPool API Calls - Unfinished
	if srv.tpool != nil {
		mux.GET("/transactionpool/transactions", srv.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if srv.wallet != nil {
		mux.GET("/wallet", srv.walletHandler)
		mux.POST("/wallet/033x", srv.wallet033xHandler)
		mux.GET("/wallet/address", srv.walletAddressHandler)
		mux.GET("/wallet/addresses", srv.walletAddressesHandler)
		mux.GET("/wallet/backup", srv.walletBackupHandler)
		mux.POST("/wallet/init", srv.walletInitHandler)
		mux.POST("/wallet/lock", srv.walletLockHandler)
		mux.POST("/wallet/seed", srv.walletSeedHandler)
		mux.GET("/wallet/seeds", srv.walletSeedsHandler)
		mux.POST("/wallet/siacoins", srv.walletSiacoinsHandler)
		mux.POST("/wallet/siafunds", srv.walletSiafundsHandler)
		mux.POST("/wallet/siagkey", srv.walletSiagkeyHandler)
		mux.GET("/wallet/transaction/:id", srv.walletTransactionHandler)
		mux.GET("/wallet/transactions", srv.walletTransactionsHandler)
		mux.GET("/wallet/transactions/:addr", srv.walletTransactionsAddrHandler)
		mux.POST("/wallet/unlock", srv.walletUnlockHandler)
		mux.POST("/wallet/encrypt", srv.walletInitHandler) // COMPATv0.4.0
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
