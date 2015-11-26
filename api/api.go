package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stretchr/graceful"
)

const (
	apiTimeout = 5 * time.Second
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

// handleHTTPRequest is a wrapper function that handles all incoming calls to
// the API.
func (srv *Server) handleHTTPRequest(mux *http.ServeMux, url string, handler http.HandlerFunc) {
	mux.HandleFunc(url, func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.UserAgent(), srv.requiredUserAgent) {
			writeError(w, "Browser access disabled due to security vulnerability. Use Sia-UI or siac.", http.StatusBadRequest)
			return
		}
		handler(w, req)
	})
}

// initAPI determines which functions handle each API call.
func (srv *Server) initAPI() {
	mux := http.NewServeMux()

	// 404 Calls
	srv.handleHTTPRequest(mux, "/", srv.unrecognizedCallHandler)

	// Daemon API Calls - Unfinished
	srv.handleHTTPRequest(mux, "/daemon/constants", srv.daemonConstantsHandler)
	srv.handleHTTPRequest(mux, "/daemon/version", srv.daemonVersionHandler)
	if !srv.limitedAPI {
		srv.handleHTTPRequest(mux, "/daemon/stop", srv.daemonStopHandler)
		srv.handleHTTPRequest(mux, "/daemon/updates/apply", srv.daemonUpdatesApplyHandler)
		srv.handleHTTPRequest(mux, "/daemon/updates/check", srv.daemonUpdatesCheckHandler)
	}

	// Consensus API Calls
	if srv.cs != nil {
		srv.handleHTTPRequest(mux, "/consensus", srv.consensusHandler)            // GET
		srv.handleHTTPRequest(mux, "/consensus/block", srv.consensusBlockHandler) // GET
	}

	// Explorer API Calls
	if srv.explorer != nil {
		srv.handleHTTPRequest(mux, "/explorer", srv.explorerHandler)            // GET
		srv.handleHTTPRequest(mux, "/explorer/", srv.explorerHandler)           // $(hash) GET
		srv.handleHTTPRequest(mux, "/explorer/block", srv.explorerBlockHandler) // GET
	}

	// Gateway API Calls - Unfinished
	if srv.gateway != nil && !srv.limitedAPI {
		srv.handleHTTPRequest(mux, "/gateway/status", srv.gatewayStatusHandler)
		srv.handleHTTPRequest(mux, "/gateway/peers/add", srv.gatewayPeersAddHandler)
		srv.handleHTTPRequest(mux, "/gateway/peers/remove", srv.gatewayPeersRemoveHandler)
	}

	// Host API Calls
	if srv.host != nil && !srv.limitedAPI {
		srv.handleHTTPRequest(mux, "/host", srv.hostHandler)                  // GET, POST
		srv.handleHTTPRequest(mux, "/host/announce", srv.hostAnnounceHandler) // POST
	}

	// HostDB API Calls - DEPRECATED
	if srv.renter != nil {
		srv.handleHTTPRequest(mux, "/hostdb/hosts/active", srv.renterHostsActiveHandler)
		srv.handleHTTPRequest(mux, "/hostdb/hosts/all", srv.renterHostsAllHandler)
	}

	// Miner API Calls
	if srv.miner != nil && !srv.limitedAPI {
		srv.handleHTTPRequest(mux, "/miner", srv.minerHandler)                            // GET
		srv.handleHTTPRequest(mux, "/miner/header", srv.minerHeaderHandler)               // GET, POST
		srv.handleHTTPRequest(mux, "/miner/start", srv.minerStartHandler)                 // POST
		srv.handleHTTPRequest(mux, "/miner/stop", srv.minerStopHandler)                   // POST
		srv.handleHTTPRequest(mux, "/miner/headerforwork", srv.minerHeaderforworkHandler) // COMPATv0.4.8
		srv.handleHTTPRequest(mux, "/miner/submitheader", srv.minerSubmitheaderHandler)   // COMPATv0.4.8
	}

	// Renter API Calls - Unfinished
	if srv.renter != nil && !srv.limitedAPI {
		srv.handleHTTPRequest(mux, "/renter/downloadqueue", srv.renterDownloadqueueHandler)
		srv.handleHTTPRequest(mux, "/renter/files/delete", srv.renterFilesDeleteHandler)
		srv.handleHTTPRequest(mux, "/renter/files/download", srv.renterFilesDownloadHandler)
		srv.handleHTTPRequest(mux, "/renter/files/list", srv.renterFilesListHandler)
		srv.handleHTTPRequest(mux, "/renter/files/load", srv.renterFilesLoadHandler)
		srv.handleHTTPRequest(mux, "/renter/files/loadascii", srv.renterFilesLoadAsciiHandler)
		srv.handleHTTPRequest(mux, "/renter/files/rename", srv.renterFilesRenameHandler)
		srv.handleHTTPRequest(mux, "/renter/files/share", srv.renterFilesShareHandler)
		srv.handleHTTPRequest(mux, "/renter/files/shareascii", srv.renterFilesShareAsciiHandler)
		srv.handleHTTPRequest(mux, "/renter/files/upload", srv.renterFilesUploadHandler)
		srv.handleHTTPRequest(mux, "/renter/status", srv.renterStatusHandler)
	}

	// TransactionPool API Calls - Unfinished
	if srv.tpool != nil && !srv.limitedAPI {
		srv.handleHTTPRequest(mux, "/transactionpool/transactions", srv.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if srv.wallet != nil && !srv.limitedAPI {
		srv.handleHTTPRequest(mux, "/wallet", srv.walletHandler)                           // GET
		srv.handleHTTPRequest(mux, "/wallet/address", srv.walletAddressHandler)            // GET
		srv.handleHTTPRequest(mux, "/wallet/addresses", srv.walletAddressesHandler)        // GET
		srv.handleHTTPRequest(mux, "/wallet/backup", srv.walletBackupHandler)              // GET
		srv.handleHTTPRequest(mux, "/wallet/encrypt", srv.walletEncryptHandler)            // POST - COMPATv0.4.0
		srv.handleHTTPRequest(mux, "/wallet/init", srv.walletInitHandler)                  // POST
		srv.handleHTTPRequest(mux, "/wallet/load/033x", srv.walletLoad033xHandler)         // POST
		srv.handleHTTPRequest(mux, "/wallet/load/seed", srv.walletLoadSeedHandler)         // POST
		srv.handleHTTPRequest(mux, "/wallet/load/siag", srv.walletLoadSiagHandler)         // POST
		srv.handleHTTPRequest(mux, "/wallet/lock", srv.walletLockHandler)                  // POST
		srv.handleHTTPRequest(mux, "/wallet/seeds", srv.walletSeedsHandler)                // GET
		srv.handleHTTPRequest(mux, "/wallet/siacoins", srv.walletSiacoinsHandler)          // POST
		srv.handleHTTPRequest(mux, "/wallet/siafunds", srv.walletSiafundsHandler)          // POST
		srv.handleHTTPRequest(mux, "/wallet/transaction/", srv.walletTransactionHandler)   // $(id) GET
		srv.handleHTTPRequest(mux, "/wallet/transactions", srv.walletTransactionsHandler)  // GET
		srv.handleHTTPRequest(mux, "/wallet/transactions/", srv.walletTransactionsHandler) // $(addr) GET
		srv.handleHTTPRequest(mux, "/wallet/unlock", srv.walletUnlockHandler)              // POST
	}

	// Create graceful HTTP server
	srv.apiServer = &graceful.Server{
		Timeout: apiTimeout,
		Server:  &http.Server{Handler: mux},
	}
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
