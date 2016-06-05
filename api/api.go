package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
)

// HttpGET is a utility function for making http get requests to sia with a
// whitelisted user-agent. A non-2xx response does not return an error.
func HttpGET(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sia-Agent")
	return new(http.Client).Do(req)
}

// HttpGETAuthenticated is a utility function for making authenticated http get
// requests to sia with a whitelisted user-agent and the supplied password. A
// non-2xx response does not return an error.
func HttpGETAuthenticated(url string, password string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sia-Agent")
	req.SetBasicAuth("", password)
	return new(http.Client).Do(req)
}

// HttpPOST is a utility function for making post requests to sia with a
// whitelisted user-agent. A non-2xx response does not return an error.
func HttpPOST(url string, data string) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sia-Agent")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return new(http.Client).Do(req)
}

// HttpPOSTAuthenticated is a utility function for making authenticated http
// post requests to sia with a whitelisted user-agent and the supplied
// password. A non-2xx response does not return an error.
func HttpPOSTAuthenticated(url string, data string, password string) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sia-Agent")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("", password)
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

// requirePassword is middleware that requires a request to authenticate with a
// password using HTTP basic auth. Usernames are ignored. Empty passwords
// indicate no authentication is required.
func requirePassword(h httprouter.Handle, password string) httprouter.Handle {
	// An empty password is equivalent to no password.
	if password == "" {
		return h
	}
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		_, pass, ok := req.BasicAuth()
		if !ok || pass != password {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"SiaAPI\"")
			writeError(w, "API authentication failed.", http.StatusUnauthorized)
			return
		}
		h(w, req, ps)
	}
}

// initAPI determines which functions handle each API call. An empty string as
// the password indicates no password.
func (srv *Server) initAPI(password string) {
	router := httprouter.New()
	router.NotFound = http.HandlerFunc(srv.unrecognizedCallHandler) // custom 404

	// Daemon API Calls
	router.GET("/daemon/constants", srv.daemonConstantsHandler)
	router.GET("/daemon/version", srv.daemonVersionHandler)
	router.GET("/daemon/stop", requirePassword(srv.daemonStopHandler, password))

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
		router.POST("/gateway/connect/:netaddress", requirePassword(srv.gatewayConnectHandler, password))
		router.POST("/gateway/disconnect/:netaddress", requirePassword(srv.gatewayDisconnectHandler, password))
		// COMPATv0.6.0
		router.POST("/gateway/add/:netaddress", requirePassword(srv.gatewayConnectHandler, password))
		router.POST("/gateway/remove/:netaddress", requirePassword(srv.gatewayDisconnectHandler, password))
	}

	// Host API Calls
	if srv.host != nil {
		// Calls directly pertaining to the host.
		router.GET("/host", srv.hostHandlerGET)                                           // Get a bunch of information about the host.
		router.POST("/host", requirePassword(srv.hostHandlerPOST, password))              // Set HostInternalSettings.
		router.POST("/host/announce", requirePassword(srv.hostAnnounceHandler, password)) // Announce the host, optionally on a specific address.

		// Calls pertaining to the storage manager that the host uses.
		router.GET("/storage", srv.storageHandler)
		router.POST("/storage/folders/add", requirePassword(srv.storageFoldersAddHandler, password))
		router.POST("/storage/folders/remove", requirePassword(srv.storageFoldersRemoveHandler, password))
		router.POST("/storage/folders/resize", requirePassword(srv.storageFoldersResizeHandler, password))
		router.POST("/storage/sectors/delete/:merkleroot", requirePassword(srv.storageSectorsDeleteHandler, password))
	}

	// Miner API Calls
	if srv.miner != nil {
		router.GET("/miner", srv.minerHandler)
		router.GET("/miner/header", requirePassword(srv.minerHeaderHandlerGET, password))
		router.POST("/miner/header", requirePassword(srv.minerHeaderHandlerPOST, password))
		router.GET("/miner/start", requirePassword(srv.minerStartHandler, password))
		router.GET("/miner/stop", requirePassword(srv.minerStopHandler, password))
	}

	// Renter API Calls
	if srv.renter != nil {
		router.GET("/renter", srv.renterHandler)
		router.GET("/renter/allowance", srv.renterAllowanceHandlerGET)
		router.POST("/renter/allowance", requirePassword(srv.renterAllowanceHandlerPOST, password))
		router.GET("/renter/contracts", srv.renterContractsHandler)
		router.GET("/renter/downloads", srv.renterDownloadsHandler)
		router.GET("/renter/files", srv.renterFilesHandler)

		router.POST("/renter/load", requirePassword(srv.renterLoadHandler, password))
		router.POST("/renter/loadascii", requirePassword(srv.renterLoadAsciiHandler, password))
		router.GET("/renter/share", requirePassword(srv.renterShareHandler, password))
		router.GET("/renter/shareascii", requirePassword(srv.renterShareAsciiHandler, password))

		router.POST("/renter/delete/*siapath", requirePassword(srv.renterDeleteHandler, password))
		router.GET("/renter/download/*siapath", requirePassword(srv.renterDownloadHandler, password))
		router.POST("/renter/rename/*siapath", requirePassword(srv.renterRenameHandler, password))
		router.POST("/renter/upload/*siapath", requirePassword(srv.renterUploadHandler, password))

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
		router.POST("/wallet/033x", requirePassword(srv.wallet033xHandler, password))
		router.GET("/wallet/address", requirePassword(srv.walletAddressHandler, password))
		router.GET("/wallet/addresses", srv.walletAddressesHandler)
		router.GET("/wallet/backup", requirePassword(srv.walletBackupHandler, password))
		router.POST("/wallet/init", requirePassword(srv.walletInitHandler, password))
		router.POST("/wallet/lock", requirePassword(srv.walletLockHandler, password))
		router.POST("/wallet/seed", requirePassword(srv.walletSeedHandler, password))
		router.GET("/wallet/seeds", requirePassword(srv.walletSeedsHandler, password))
		router.POST("/wallet/siacoins", requirePassword(srv.walletSiacoinsHandler, password))
		router.POST("/wallet/siafunds", requirePassword(srv.walletSiafundsHandler, password))
		router.POST("/wallet/siagkey", requirePassword(srv.walletSiagkeyHandler, password))
		router.GET("/wallet/transaction/:id", srv.walletTransactionHandler)
		router.GET("/wallet/transactions", srv.walletTransactionsHandler)
		router.GET("/wallet/transactions/:addr", srv.walletTransactionsAddrHandler)
		router.POST("/wallet/unlock", requirePassword(srv.walletUnlockHandler, password))
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
