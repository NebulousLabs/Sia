package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/julienschmidt/httprouter"
)

// buildHttpRoutes sets up and returns an * httprouter.Router.
// it connected the Router to the given api using the required
// parameters: requiredUserAgent and requiredPassword
func (api *API) buildHTTPRoutes(requiredUserAgent string, requiredPassword string) {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(UnrecognizedCallHandler)
	router.RedirectTrailingSlash = false

	// Consensus API Calls
	if api.cs != nil {
		router.GET("/consensus", api.consensusHandler)
		router.GET("/consensus/blocks", api.consensusBlocksHandler)
		router.POST("/consensus/validate/transactionset", api.consensusValidateTransactionsetHandler)
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
		router.POST("/gateway/connect/:netaddress", RequirePassword(api.gatewayConnectHandler, requiredPassword))
		router.POST("/gateway/disconnect/:netaddress", RequirePassword(api.gatewayDisconnectHandler, requiredPassword))
	}

	// Host API Calls
	if api.host != nil {
		// Calls directly pertaining to the host.
		router.GET("/host", api.hostHandlerGET)                                                   // Get the host status.
		router.POST("/host", RequirePassword(api.hostHandlerPOST, requiredPassword))              // Change the settings of the host.
		router.POST("/host/announce", RequirePassword(api.hostAnnounceHandler, requiredPassword)) // Announce the host to the network.
		router.GET("/host/contracts", api.hostContractInfoHandler)                                // Get info about contracts.
		router.GET("/host/estimatescore", api.hostEstimateScoreGET)

		// Calls pertaining to the storage manager that the host uses.
		router.GET("/host/storage", api.storageHandler)
		router.POST("/host/storage/folders/add", RequirePassword(api.storageFoldersAddHandler, requiredPassword))
		router.POST("/host/storage/folders/remove", RequirePassword(api.storageFoldersRemoveHandler, requiredPassword))
		router.POST("/host/storage/folders/resize", RequirePassword(api.storageFoldersResizeHandler, requiredPassword))
		router.POST("/host/storage/sectors/delete/:merkleroot", RequirePassword(api.storageSectorsDeleteHandler, requiredPassword))
	}

	// Miner API Calls
	if api.miner != nil {
		router.GET("/miner", api.minerHandler)
		router.GET("/miner/header", RequirePassword(api.minerHeaderHandlerGET, requiredPassword))
		router.POST("/miner/header", RequirePassword(api.minerHeaderHandlerPOST, requiredPassword))
		router.GET("/miner/start", RequirePassword(api.minerStartHandler, requiredPassword))
		router.GET("/miner/stop", RequirePassword(api.minerStopHandler, requiredPassword))
	}

	// Renter API Calls
	if api.renter != nil {
		router.GET("/renter", api.renterHandlerGET)
		router.POST("/renter", RequirePassword(api.renterHandlerPOST, requiredPassword))
		router.GET("/renter/contracts", api.renterContractsHandler)
		router.GET("/renter/downloads", api.renterDownloadsHandler)
		router.GET("/renter/files", api.renterFilesHandler)
		router.GET("/renter/file/*siapath", api.renterFileHandler)
		router.GET("/renter/prices", api.renterPricesHandler)

		// TODO: re-enable these routes once the new .sia format has been
		// standardized and implemented.
		// router.POST("/renter/load", RequirePassword(api.renterLoadHandler, requiredPassword))
		// router.POST("/renter/loadascii", RequirePassword(api.renterLoadAsciiHandler, requiredPassword))
		// router.GET("/renter/share", RequirePassword(api.renterShareHandler, requiredPassword))
		// router.GET("/renter/shareascii", RequirePassword(api.renterShareAsciiHandler, requiredPassword))

		router.POST("/renter/delete/*siapath", RequirePassword(api.renterDeleteHandler, requiredPassword))
		router.GET("/renter/download/*siapath", RequirePassword(api.renterDownloadHandler, requiredPassword))
		router.GET("/renter/downloadasync/*siapath", RequirePassword(api.renterDownloadAsyncHandler, requiredPassword))
		router.POST("/renter/rename/*siapath", RequirePassword(api.renterRenameHandler, requiredPassword))
		router.GET("/renter/stream/*siapath", api.renterStreamHandler)
		router.POST("/renter/upload/*siapath", RequirePassword(api.renterUploadHandler, requiredPassword))

		// HostDB endpoints.
		router.GET("/hostdb", api.hostdbHandler)
		router.GET("/hostdb/active", api.hostdbActiveHandler)
		router.GET("/hostdb/all", api.hostdbAllHandler)
		router.GET("/hostdb/hosts/:pubkey", api.hostdbHostsHandler)
	}

	// Transaction pool API Calls
	if api.tpool != nil {
		router.GET("/tpool/fee", api.tpoolFeeHandlerGET)
		router.GET("/tpool/raw/:id", api.tpoolRawHandlerGET)
		router.POST("/tpool/raw", api.tpoolRawHandlerPOST)
		router.GET("/tpool/confirmed/:id", api.tpoolConfirmedGET)

		// TODO: re-enable this route once the transaction pool API has been finalized
		//router.GET("/transactionpool/transactions", api.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if api.wallet != nil {
		router.GET("/wallet", api.walletHandler)
		router.POST("/wallet/033x", RequirePassword(api.wallet033xHandler, requiredPassword))
		router.GET("/wallet/address", RequirePassword(api.walletAddressHandler, requiredPassword))
		router.GET("/wallet/addresses", api.walletAddressesHandler)
		router.GET("/wallet/backup", RequirePassword(api.walletBackupHandler, requiredPassword))
		router.POST("/wallet/init", RequirePassword(api.walletInitHandler, requiredPassword))
		router.POST("/wallet/init/seed", RequirePassword(api.walletInitSeedHandler, requiredPassword))
		router.POST("/wallet/lock", RequirePassword(api.walletLockHandler, requiredPassword))
		router.POST("/wallet/seed", RequirePassword(api.walletSeedHandler, requiredPassword))
		router.GET("/wallet/seeds", RequirePassword(api.walletSeedsHandler, requiredPassword))
		router.POST("/wallet/siacoins", RequirePassword(api.walletSiacoinsHandler, requiredPassword))
		router.POST("/wallet/siafunds", RequirePassword(api.walletSiafundsHandler, requiredPassword))
		router.POST("/wallet/siagkey", RequirePassword(api.walletSiagkeyHandler, requiredPassword))
		router.POST("/wallet/sweep/seed", RequirePassword(api.walletSweepSeedHandler, requiredPassword))
		router.GET("/wallet/transaction/:id", api.walletTransactionHandler)
		router.GET("/wallet/transactions", api.walletTransactionsHandler)
		router.GET("/wallet/transactions/:addr", api.walletTransactionsAddrHandler)
		router.GET("/wallet/verify/address/:addr", api.walletVerifyAddressHandler)
		router.POST("/wallet/unlock", RequirePassword(api.walletUnlockHandler, requiredPassword))
		router.POST("/wallet/changepassword", RequirePassword(api.walletChangePasswordHandler, requiredPassword))
	}

	// Apply UserAgent middleware and return the Router
	api.router = cleanCloseHandler(RequireUserAgent(router, requiredUserAgent))
	return
}

// cleanCloseHandler wraps the entire API, ensuring that underlying conns are
// not leaked if the remote end closes the connection before the underlying
// handler finishes.
func cleanCloseHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close this file handle either when the function completes or when the
		// connection is done.
		done := make(chan struct{})
		go func(w http.ResponseWriter, r *http.Request) {
			defer close(done)
			next.ServeHTTP(w, r)
		}(w, r)
		select {
		case <-done:
		}

		// Sanity check - thread should not take more than an hour to return. This
		// must be done in a goroutine, otherwise the server will not close the
		// underlying socket for this API call.
		timer := time.NewTimer(time.Minute * 60)
		go func() {
			select {
			case <-done:
				timer.Stop()
			case <-timer.C:
				build.Severe("api call is taking more than 60 minutes to return:", r.URL.Path)
			}
		}()
	})
}

// RequireUserAgent is middleware that requires all requests to set a
// UserAgent that contains the specified string.
func RequireUserAgent(h http.Handler, ua string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.UserAgent(), ua) && !isUnrestricted(req) {
			WriteError(w, Error{"Browser access disabled due to security vulnerability. Use Sia-UI or siac."}, http.StatusBadRequest)
			return
		}
		h.ServeHTTP(w, req)
	})
}

// RequirePassword is middleware that requires a request to authenticate with a
// password using HTTP basic auth. Usernames are ignored. Empty passwords
// indicate no authentication is required.
func RequirePassword(h httprouter.Handle, password string) httprouter.Handle {
	// An empty password is equivalent to no password.
	if password == "" {
		return h
	}
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		_, pass, ok := req.BasicAuth()
		if !ok || pass != password {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"SiaAPI\"")
			WriteError(w, Error{"API authentication failed."}, http.StatusUnauthorized)
			return
		}
		h(w, req, ps)
	}
}

// isUnrestricted checks if a request may bypass the useragent check.
func isUnrestricted(req *http.Request) bool {
	return strings.HasPrefix(req.URL.Path, "/renter/stream/")
}
