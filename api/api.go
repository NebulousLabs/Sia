package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/modules"

	"github.com/julienschmidt/httprouter"
)

// Error is a type that is encoded as JSON and returned in an API response in
// the event of an error. Only the Message field is required. More fields may
// be added to this struct in the future for better error reporting.
type Error struct {
	// Message describes the error in English. Typically it is set to
	// `err.Error()`. This field is required.
	Message string `json:"message"`

	// TODO: add a Param field with the (omitempty option in the json tag)
	// to indicate that the error was caused by an invalid, missing, or
	// incorrect parameter. This is not trivial as the API does not
	// currently do parameter validation itself. For example, the
	// /gateway/connect endpoint relies on the gateway.Connect method to
	// validate the netaddress. However, this prevents the API from knowing
	// whether an error returned by gateway.Connect is because of a
	// connection error or an invalid netaddress parameter. Validating
	// parameters in the API is not sufficient, as a parameter's value may
	// be valid or invalid depending on the current state of a module.
}

// Error implements the error interface for the Error type. It returns only the
// Message field.
func (err Error) Error() string {
	return err.Message
}

// HttpGET is a utility function for making http get requests to sia with a
// whitelisted user-agent. A non-2xx response does not return an error.
func HttpGET(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sia-Agent")
	return http.DefaultClient.Do(req)
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
	return http.DefaultClient.Do(req)
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
	return http.DefaultClient.Do(req)
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
	return http.DefaultClient.Do(req)
}

// requireUserAgent is middleware that requires all requests to set a
// UserAgent that contains the specified string.
func requireUserAgent(h http.Handler, ua string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.UserAgent(), ua) {
			writeError(w, Error{"Browser access disabled due to security vulnerability. Use Sia-UI or siac."}, http.StatusBadRequest)
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
			writeError(w, Error{"API authentication failed."}, http.StatusUnauthorized)
			return
		}
		h(w, req, ps)
	}
}

// API encapsulates a collection of modules and implements a http.Handler
// to access their methods.
type API struct {
	cs       modules.ConsensusSet
	explorer modules.Explorer
	gateway  modules.Gateway
	host     modules.Host
	miner    modules.Miner
	renter   modules.Renter
	tpool    modules.TransactionPool
	wallet   modules.Wallet

	router http.Handler
}

// api.ServeHTTP implements the http.Handler interface.
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	api.router.ServeHTTP(w, r)
}

// New creates a new Sia API from the provided modules.
// The API will require authentication using HTTP basic auth for certain endpoints
// if the suppliecd password is not the empty string.  Usernames are ignored for authentication.
func New(requiredUserAgent string, requiredPassword string, cs modules.ConsensusSet, e modules.Explorer, g modules.Gateway, h modules.Host, m modules.Miner, r modules.Renter, tp modules.TransactionPool, w modules.Wallet) *API {
	api := &API{
		cs:       cs,
		explorer: e,
		gateway:  g,
		host:     h,
		miner:    m,
		renter:   r,
		tpool:    tp,
		wallet:   w,
	}

	// Register API handlers
	router := httprouter.New()
	router.NotFound = http.HandlerFunc(api.unrecognizedCallHandler)

	// Consensus API Calls
	if api.cs != nil {
		router.GET("/consensus", api.consensusHandler)
	}

	// Explorer API Calls
	if api.explorer != nil {
		router.GET("/explorer", api.explorerHandler)
		router.GET("/explorer/blocks/:height", api.explorerBlocksHandler)
		router.GET("/explorer/hashes/:hash", api.explorerHashHandler)
	}

	// Gateway API Calls
	if api.gateway != nil {
		router.GET("/gateway", api.gatewayHandler)
		router.POST("/gateway/connect/:netaddress", requirePassword(api.gatewayConnectHandler, requiredPassword))
		router.POST("/gateway/disconnect/:netaddress", requirePassword(api.gatewayDisconnectHandler, requiredPassword))
	}

	// Host API Calls
	if api.host != nil {
		// Calls directly pertaining to the host.
		router.GET("/host", api.hostHandlerGET)                                                   // Get the host status.
		router.POST("/host", requirePassword(api.hostHandlerPOST, requiredPassword))              // Change the settings of the host.
		router.POST("/host/announce", requirePassword(api.hostAnnounceHandler, requiredPassword)) // Announce the host to the network.

		// Calls pertaining to the storage manager that the host uses.
		router.GET("/host/storage", api.storageHandler)
		router.POST("/host/storage/folders/add", requirePassword(api.storageFoldersAddHandler, requiredPassword))
		router.POST("/host/storage/folders/remove", requirePassword(api.storageFoldersRemoveHandler, requiredPassword))
		router.POST("/host/storage/folders/resize", requirePassword(api.storageFoldersResizeHandler, requiredPassword))
		router.POST("/host/storage/sectors/delete/:merkleroot", requirePassword(api.storageSectorsDeleteHandler, requiredPassword))
	}

	// Miner API Calls
	if api.miner != nil {
		router.GET("/miner", api.minerHandler)
		router.GET("/miner/header", requirePassword(api.minerHeaderHandlerGET, requiredPassword))
		router.POST("/miner/header", requirePassword(api.minerHeaderHandlerPOST, requiredPassword))
		router.GET("/miner/start", requirePassword(api.minerStartHandler, requiredPassword))
		router.GET("/miner/stop", requirePassword(api.minerStopHandler, requiredPassword))
	}

	// Renter API Calls
	if api.renter != nil {
		router.GET("/renter", api.renterHandlerGET)
		router.POST("/renter", requirePassword(api.renterHandlerPOST, requiredPassword))
		router.GET("/renter/contracts", api.renterContractsHandler)
		router.GET("/renter/downloads", api.renterDownloadsHandler)
		router.GET("/renter/files", api.renterFilesHandler)

		// TODO: re-enable these routes once the new .sia format has been
		// standardized and implemented.
		// router.POST("/renter/load", requirePassword(api.renterLoadHandler, requiredPassword))
		// router.POST("/renter/loadascii", requirePassword(api.renterLoadAsciiHandler, requiredPassword))
		// router.GET("/renter/share", requirePassword(api.renterShareHandler, requiredPassword))
		// router.GET("/renter/shareascii", requirePassword(api.renterShareAsciiHandler, requiredPassword))

		router.POST("/renter/delete/*siapath", requirePassword(api.renterDeleteHandler, requiredPassword))
		router.GET("/renter/download/*siapath", requirePassword(api.renterDownloadHandler, requiredPassword))
		router.POST("/renter/rename/*siapath", requirePassword(api.renterRenameHandler, requiredPassword))
		router.POST("/renter/upload/*siapath", requirePassword(api.renterUploadHandler, requiredPassword))

		// HostDB endpoints.
		router.GET("/hostdb/active", api.renterHostsActiveHandler)
		router.GET("/hostdb/all", api.renterHostsAllHandler)
	}

	// TransactionPool API Calls
	if api.tpool != nil {
		// TODO: re-enable this route once the transaction pool API has been finalized
		//router.GET("/transactionpool/transactions", api.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if api.wallet != nil {
		router.GET("/wallet", api.walletHandler)
		router.POST("/wallet/033x", requirePassword(api.wallet033xHandler, requiredPassword))
		router.GET("/wallet/address", requirePassword(api.walletAddressHandler, requiredPassword))
		router.GET("/wallet/addresses", api.walletAddressesHandler)
		router.GET("/wallet/backup", requirePassword(api.walletBackupHandler, requiredPassword))
		router.POST("/wallet/init", requirePassword(api.walletInitHandler, requiredPassword))
		router.POST("/wallet/lock", requirePassword(api.walletLockHandler, requiredPassword))
		router.POST("/wallet/seed", requirePassword(api.walletSeedHandler, requiredPassword))
		router.GET("/wallet/seeds", requirePassword(api.walletSeedsHandler, requiredPassword))
		router.POST("/wallet/siacoins", requirePassword(api.walletSiacoinsHandler, requiredPassword))
		router.POST("/wallet/siafunds", requirePassword(api.walletSiafundsHandler, requiredPassword))
		router.POST("/wallet/siagkey", requirePassword(api.walletSiagkeyHandler, requiredPassword))
		router.GET("/wallet/transaction/:id", api.walletTransactionHandler)
		router.GET("/wallet/transactions", api.walletTransactionsHandler)
		router.GET("/wallet/transactions/:addr", api.walletTransactionsAddrHandler)
		router.POST("/wallet/unlock", requirePassword(api.walletUnlockHandler, requiredPassword))
	}

	// Apply UserAgent middleware and return the API
	api.router = requireUserAgent(router, requiredUserAgent)
	return api
}

// unrecognizedCallHandler handles calls to unknown pages (404).
func (api *API) unrecognizedCallHandler(w http.ResponseWriter, req *http.Request) {
	writeError(w, Error{"404 - Refer to API.md"}, http.StatusNotFound)
}

// writeError an error to the API caller.
func writeError(w http.ResponseWriter, err Error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if json.NewEncoder(w).Encode(err) != nil {
		http.Error(w, "Failed to encode error response", http.StatusInternalServerError)
	}
}

// writeJSON writes the object to the ResponseWriter. If the encoding fails, an
// error is written instead. The Content-Type of the response header is set
// accordingly.
func writeJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if json.NewEncoder(w).Encode(obj) != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// writeSuccess writes the HTTP header with status 204 No Content to the
// ResponseWriter. writeSuccess should only be used to indicate that the
// requested action succeeded AND there is no data to return.
func writeSuccess(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
