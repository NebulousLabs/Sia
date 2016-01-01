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
	mux := http.NewServeMux()

	// 404 Calls
	mux.HandleFunc("/", srv.unrecognizedCallHandler)

	// Daemon API Calls - Unfinished
	mux.HandleFunc("/daemon/constants", srv.daemonConstantsHandler)
	mux.HandleFunc("/daemon/version", srv.daemonVersionHandler)
	mux.HandleFunc("/daemon/stop", srv.daemonStopHandler)
	mux.HandleFunc("/daemon/updates/apply", srv.daemonUpdatesApplyHandler)
	mux.HandleFunc("/daemon/updates/check", srv.daemonUpdatesCheckHandler)

	// Consensus API Calls
	if srv.cs != nil {
		mux.HandleFunc("/consensus", srv.consensusHandler)            // GET
		mux.HandleFunc("/consensus/block", srv.consensusBlockHandler) // GET
	}

	// Explorer API Calls
	if srv.explorer != nil {
		mux.HandleFunc("/explorer", srv.explorerHandler)            // GET
		mux.HandleFunc("/explorer/", srv.explorerHandler)           // $(hash) GET
		mux.HandleFunc("/explorer/block", srv.explorerBlockHandler) // GET
	}

	// Gateway API Calls - Unfinished
	if srv.gateway != nil {
		mux.HandleFunc("/gateway/status", srv.gatewayStatusHandler)
		mux.HandleFunc("/gateway/peers/add", srv.gatewayPeersAddHandler)
		mux.HandleFunc("/gateway/peers/remove", srv.gatewayPeersRemoveHandler)
	}

	// Host API Calls
	if srv.host != nil {
		mux.HandleFunc("/host", srv.hostHandler)                  // GET, POST
		mux.HandleFunc("/host/announce", srv.hostAnnounceHandler) // POST
	}

	// HostDB API Calls - DEPRECATED
	if srv.renter != nil {
		mux.HandleFunc("/hostdb/hosts/active", srv.renterHostsActiveHandler)
		mux.HandleFunc("/hostdb/hosts/all", srv.renterHostsAllHandler)
	}

	// Miner API Calls
	if srv.miner != nil {
		mux.HandleFunc("/miner", srv.minerHandler)                            // GET
		mux.HandleFunc("/miner/header", srv.minerHeaderHandler)               // GET, POST
		mux.HandleFunc("/miner/start", srv.minerStartHandler)                 // POST
		mux.HandleFunc("/miner/stop", srv.minerStopHandler)                   // POST
		mux.HandleFunc("/miner/headerforwork", srv.minerHeaderforworkHandler) // COMPATv0.4.8
		mux.HandleFunc("/miner/submitheader", srv.minerSubmitheaderHandler)   // COMPATv0.4.8
	}

	// Renter API Calls - Unfinished
	if srv.renter != nil {
		mux.HandleFunc("/renter/downloadqueue", srv.renterDownloadqueueHandler)
		mux.HandleFunc("/renter/files/delete", srv.renterFilesDeleteHandler)
		mux.HandleFunc("/renter/files/download", srv.renterFilesDownloadHandler)
		mux.HandleFunc("/renter/files/list", srv.renterFilesListHandler)
		mux.HandleFunc("/renter/files/load", srv.renterFilesLoadHandler)
		mux.HandleFunc("/renter/files/loadascii", srv.renterFilesLoadAsciiHandler)
		mux.HandleFunc("/renter/files/rename", srv.renterFilesRenameHandler)
		mux.HandleFunc("/renter/files/share", srv.renterFilesShareHandler)
		mux.HandleFunc("/renter/files/shareascii", srv.renterFilesShareAsciiHandler)
		mux.HandleFunc("/renter/files/upload", srv.renterFilesUploadHandler)
		mux.HandleFunc("/renter/status", srv.renterStatusHandler)
	}

	// TransactionPool API Calls - Unfinished
	if srv.tpool != nil {
		mux.HandleFunc("/transactionpool/transactions", srv.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if srv.wallet != nil {
		mux.HandleFunc("/wallet", srv.walletHandler)                           // GET
		mux.HandleFunc("/wallet/address", srv.walletAddressHandler)            // GET
		mux.HandleFunc("/wallet/addresses", srv.walletAddressesHandler)        // GET
		mux.HandleFunc("/wallet/backup", srv.walletBackupHandler)              // GET
		mux.HandleFunc("/wallet/encrypt", srv.walletEncryptHandler)            // POST - COMPATv0.4.0
		mux.HandleFunc("/wallet/init", srv.walletInitHandler)                  // POST
		mux.HandleFunc("/wallet/load/033x", srv.walletLoad033xHandler)         // POST
		mux.HandleFunc("/wallet/load/seed", srv.walletLoadSeedHandler)         // POST
		mux.HandleFunc("/wallet/load/siag", srv.walletLoadSiagHandler)         // POST
		mux.HandleFunc("/wallet/lock", srv.walletLockHandler)                  // POST
		mux.HandleFunc("/wallet/seeds", srv.walletSeedsHandler)                // GET
		mux.HandleFunc("/wallet/siacoins", srv.walletSiacoinsHandler)          // POST
		mux.HandleFunc("/wallet/siafunds", srv.walletSiafundsHandler)          // POST
		mux.HandleFunc("/wallet/transaction/", srv.walletTransactionHandler)   // $(id) GET
		mux.HandleFunc("/wallet/transactions", srv.walletTransactionsHandler)  // GET
		mux.HandleFunc("/wallet/transactions/", srv.walletTransactionsHandler) // $(addr) GET
		mux.HandleFunc("/wallet/unlock", srv.walletUnlockHandler)              // POST
	}

	// Create graceful HTTP server
	uaMux := requireUserAgent(mux, srv.requiredUserAgent)
	srv.apiServer = &graceful.Server{
		Timeout: apiTimeout,
		Server:  &http.Server{Handler: uaMux},
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
