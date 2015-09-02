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

// handleHTTPRequest is a wrapper function that handles all incoming calls to
// the API.
func handleHTTPRequest(mux *http.ServeMux, url string, handler http.HandlerFunc) {
	mux.HandleFunc(url, func(w http.ResponseWriter, req *http.Request) {
		// prevent access from sources other than siac and Sia-UI
		if !strings.Contains(req.UserAgent(), "Sia-Agent") && !strings.Contains(req.UserAgent(), "Sia-Miner") && req.UserAgent() != "" && req.UserAgent() != "Go 1.1 package http" && !strings.Contains(req.UserAgent(), "Electron") && !strings.Contains(req.UserAgent(), "AtomShell") {
			writeError(w, "Browser access disabled due to security vulnerability. Use Sia-UI or siac.", http.StatusInternalServerError)
			return
		}
		handler(w, req)
	})
}

// initAPI determines which functions handle each API call.
func (srv *Server) initAPI(addr string) {
	mux := http.NewServeMux()

	// 404 Calls
	handleHTTPRequest(mux, "/", srv.unrecognizedCallHandler)

	// Daemon API Calls - Unfinished
	handleHTTPRequest(mux, "/daemon/constants", srv.daemonConstantsHandler)
	handleHTTPRequest(mux, "/daemon/stop", srv.daemonStopHandler)
	handleHTTPRequest(mux, "/daemon/version", srv.daemonVersionHandler)
	handleHTTPRequest(mux, "/daemon/updates/apply", srv.daemonUpdatesApplyHandler)
	handleHTTPRequest(mux, "/daemon/updates/check", srv.daemonUpdatesCheckHandler)

	// Consensus API Calls
	if srv.cs != nil {
		handleHTTPRequest(mux, "/consensus", srv.consensusHandler) // GET
	}

	// Gateway API Calls - Unfinished
	if srv.gateway != nil {
		handleHTTPRequest(mux, "/gateway/status", srv.gatewayStatusHandler)
		handleHTTPRequest(mux, "/gateway/peers/add", srv.gatewayPeersAddHandler)
		handleHTTPRequest(mux, "/gateway/peers/remove", srv.gatewayPeersRemoveHandler)
	}

	// Host API Calls - Unfinished
	if srv.host != nil {
		handleHTTPRequest(mux, "/host/announce", srv.hostAnnounceHandler)
		handleHTTPRequest(mux, "/host/configure", srv.hostConfigureHandler)
		handleHTTPRequest(mux, "/host/status", srv.hostStatusHandler)
	}

	// HostDB API Calls - Unfinished
	if srv.hostdb != nil {
		handleHTTPRequest(mux, "/hostdb/hosts/active", srv.hostdbHostsActiveHandler)
		handleHTTPRequest(mux, "/hostdb/hosts/all", srv.hostdbHostsAllHandler)
	}

	// Miner API Calls - Unfinished
	if srv.miner != nil {
		handleHTTPRequest(mux, "/miner/start", srv.minerStartHandler)
		handleHTTPRequest(mux, "/miner/status", srv.minerStatusHandler)
		handleHTTPRequest(mux, "/miner/stop", srv.minerStopHandler)
		handleHTTPRequest(mux, "/miner/blockforwork", srv.minerBlockforworkHandler)
		handleHTTPRequest(mux, "/miner/submitblock", srv.minerSubmitblockHandler)
		handleHTTPRequest(mux, "/miner/headerforwork", srv.minerHeaderforworkHandler)
		handleHTTPRequest(mux, "/miner/submitheader", srv.minerSubmitheaderHandler)
	}

	// Renter API Calls - Unfinished
	if srv.renter != nil {
		handleHTTPRequest(mux, "/renter/downloadqueue", srv.renterDownloadqueueHandler)
		handleHTTPRequest(mux, "/renter/files/delete", srv.renterFilesDeleteHandler)
		handleHTTPRequest(mux, "/renter/files/download", srv.renterFilesDownloadHandler)
		handleHTTPRequest(mux, "/renter/files/list", srv.renterFilesListHandler)
		handleHTTPRequest(mux, "/renter/files/load", srv.renterFilesLoadHandler)
		handleHTTPRequest(mux, "/renter/files/loadascii", srv.renterFilesLoadAsciiHandler)
		handleHTTPRequest(mux, "/renter/files/rename", srv.renterFilesRenameHandler)
		handleHTTPRequest(mux, "/renter/files/share", srv.renterFilesShareHandler)
		handleHTTPRequest(mux, "/renter/files/shareascii", srv.renterFilesShareAsciiHandler)
		handleHTTPRequest(mux, "/renter/files/upload", srv.renterFilesUploadHandler)
		handleHTTPRequest(mux, "/renter/status", srv.renterStatusHandler)
	}

	// TransactionPool API Calls - Unfinished
	if srv.tpool != nil {
		handleHTTPRequest(mux, "/transactionpool/transactions", srv.transactionpoolTransactionsHandler)
	}

	// Wallet API Calls
	if srv.wallet != nil {
		handleHTTPRequest(mux, "/wallet", srv.walletHandler)                           // GET
		handleHTTPRequest(mux, "/wallet/address", srv.walletAddressHandler)            // GET
		handleHTTPRequest(mux, "/wallet/addresses", srv.walletAddressesHandler)        // GET
		handleHTTPRequest(mux, "/wallet/backup", srv.walletBackupHandler)              // POST
		handleHTTPRequest(mux, "/wallet/encrypt", srv.walletEncryptHandler)            // POST - COMPATv0.4.0
		handleHTTPRequest(mux, "/wallet/init", srv.walletInitHandler)                  // POST
		handleHTTPRequest(mux, "/wallet/load/033x", srv.walletLoad033xHandler)         // POST
		handleHTTPRequest(mux, "/wallet/load/seed", srv.walletLoadSeedHandler)         // POST
		handleHTTPRequest(mux, "/wallet/lock", srv.walletLockHandler)                  // POST
		handleHTTPRequest(mux, "/wallet/seeds", srv.walletSeedsHandler)                // GET
		handleHTTPRequest(mux, "/wallet/siacoins", srv.walletSiacoinsHandler)          // POST
		handleHTTPRequest(mux, "/wallet/siafunds", srv.walletSiafundsHandler)          // POST
		handleHTTPRequest(mux, "/wallet/transaction/", srv.walletTransactionHandler)   // $(id) GET
		handleHTTPRequest(mux, "/wallet/transactions", srv.walletTransactionsHandler)  // GET
		handleHTTPRequest(mux, "/wallet/transactions/", srv.walletTransactionsHandler) // $(addr) GET
		handleHTTPRequest(mux, "/wallet/unlock", srv.walletUnlockHandler)              // POST
	}

	// BlockExplorer API Calls - Unfinished
	if srv.exp != nil {
		handleHTTPRequest(mux, "/explorer/status", srv.explorerStatusHandler)
		handleHTTPRequest(mux, "/explorer/blockdata", srv.explorerBlockDataHandler)
		handleHTTPRequest(mux, "/explorer/gethash", srv.explorerGetHashHandler)
	}

	// Create graceful HTTP server
	srv.apiServer = &graceful.Server{
		Timeout: apiTimeout,
		Server:  &http.Server{Addr: addr, Handler: mux},
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
