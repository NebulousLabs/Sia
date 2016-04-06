package api

import (
	"encoding/json"
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
	router := httprouter.New()
	router.NotFound = http.HandlerFunc(srv.unrecognizedCallHandler) // custom 404

	// Daemon API Calls
	router.GET("/daemon/constants", srv.daemonConstantsHandler)
	router.GET("/daemon/version", srv.daemonVersionHandler)
	router.GET("/daemon/stop", srv.daemonStopHandler) // COMPATv0.5.2
	router.POST("/daemon/stop", srv.daemonStopHandler)

	// Consensus API Calls
	if srv.cs != nil {
		router.GET("/consensus", srv.consensusHandler)
	}

	// Explorer API Calls
	if srv.explorer != nil {
		router.GET("/explorer", srv.explorerHandler)
		router.GET("/explorer/blocks/:height", srv.explorerBlocksHandler)
		router.GET("/explorer/hashes/:hash", srv.explorerHashHandler)
	}

	// Gateway API Calls
	if srv.gateway != nil {
		router.GET("/gateway", srv.gatewayHandler)
		router.POST("/gateway/add/:netaddress", srv.gatewayAddHandler)
		router.POST("/gateway/remove/:netaddress", srv.gatewayRemoveHandler)
	}

	// Host API Calls
	if srv.host != nil {
		router.GET("/host", srv.hostHandlerGET)
		router.POST("/host", srv.hostHandlerPOST)
		router.POST("/host/announce", srv.hostAnnounceHandler)
		router.POST("/host/delete/:filecontractid", srv.hostDeleteHandler)
	}

	// Miner API Calls
	if srv.miner != nil {
		router.GET("/miner", srv.minerHandler)
		router.GET("/miner/header", srv.minerHeaderHandlerGET)
		router.POST("/miner/header", srv.minerHeaderHandlerPOST)
		router.GET("/miner/start", srv.minerStartHandler) // COMPATv0.5.2
		router.GET("/miner/stop", srv.minerStopHandler)   // COMPATv0.5.2
		router.POST("/miner/start", srv.minerStartHandler)
		router.POST("/miner/stop", srv.minerStopHandler)
		router.GET("/miner/headerforwork", srv.minerHeaderHandlerGET)  // COMPATv0.4.8
		router.POST("/miner/submitheader", srv.minerHeaderHandlerPOST) // COMPATv0.4.8
	}

	// Renter API Calls
	if srv.renter != nil {
		router.GET("/renter/downloads", srv.renterDownloadsHandler)
		router.GET("/renter/files", srv.renterFilesHandler)

		router.POST("/renter/load", srv.renterLoadHandler)
		router.POST("/renter/loadascii", srv.renterLoadAsciiHandler)
		router.GET("/renter/share", srv.renterShareHandler)
		router.GET("/renter/shareascii", srv.renterShareAsciiHandler)

		router.POST("/renter/delete/*siapath", srv.renterDeleteHandler)
		router.GET("/renter/download/*siapath", srv.renterDownloadHandler)
		router.POST("/renter/rename/*siapath", srv.renterRenameHandler)
		router.POST("/renter/upload/*siapath", srv.renterUploadHandler)

		router.GET("/renter/hosts/active", srv.renterHostsActiveHandler)
		router.GET("/renter/hosts/all", srv.renterHostsAllHandler)
	}

	// TransactionPool API Calls
	if srv.tpool != nil {
		router.GET("/transactionpool/transactions", srv.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if srv.wallet != nil {
		router.GET("/wallet", srv.walletHandler)
		router.POST("/wallet/033x", srv.wallet033xHandler)
		router.GET("/wallet/address", srv.walletAddressHandler)
		router.GET("/wallet/addresses", srv.walletAddressesHandler)
		router.GET("/wallet/backup", srv.walletBackupHandler)
		router.POST("/wallet/init", srv.walletInitHandler)
		router.POST("/wallet/lock", srv.walletLockHandler)
		router.POST("/wallet/seed", srv.walletSeedHandler)
		router.GET("/wallet/seeds", srv.walletSeedsHandler)
		router.POST("/wallet/siacoins", srv.walletSiacoinsHandler)
		router.POST("/wallet/siafunds", srv.walletSiafundsHandler)
		router.POST("/wallet/siagkey", srv.walletSiagkeyHandler)
		router.GET("/wallet/transaction/:id", srv.walletTransactionHandler)
		router.GET("/wallet/transactions", srv.walletTransactionsHandler)
		router.GET("/wallet/transactions/:addr", srv.walletTransactionsAddrHandler)
		router.POST("/wallet/unlock", srv.walletUnlockHandler)
		router.POST("/wallet/encrypt", srv.walletInitHandler) // COMPATv0.4.0
	}

	// Apply UserAgent middleware and create HTTP server
	uaRouter := requireUserAgent(router, srv.requiredUserAgent)
	srv.apiServer = &http.Server{Handler: uaRouter}
}

// unrecognizedCallHandler handles calls to unknown pages (404).
func (srv *Server) unrecognizedCallHandler(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "404 - Refer to API.md", http.StatusNotFound)
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
