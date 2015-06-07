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
		handler(w, req)
	})
}

// initAPI determines which functions handle each API call.
func (srv *Server) initAPI(addr string) {
	mux := http.NewServeMux()

	// 404 Calls
	handleHTTPRequest(mux, "/", srv.unrecognizedCallHandler)

	// Consensus API Calls
	handleHTTPRequest(mux, "/consensus/status", srv.consensusStatusHandler)
	handleHTTPRequest(mux, "/consensus/synchronize", srv.consensusSynchronizeHandler)

	// Daemon API Calls
	handleHTTPRequest(mux, "/daemon/stop", srv.daemonStopHandler)
	handleHTTPRequest(mux, "/daemon/updates/apply", srv.daemonUpdatesApplyHandler)
	handleHTTPRequest(mux, "/daemon/updates/check", srv.daemonUpdatesCheckHandler)

	// Debugging API Calls
	handleHTTPRequest(mux, "/debug/constants", srv.debugConstantsHandler)
	handleHTTPRequest(mux, "/debug/mutextest", srv.mutexTestHandler)

	// Gateway API Calls
	handleHTTPRequest(mux, "/gateway/status", srv.gatewayStatusHandler)
	handleHTTPRequest(mux, "/gateway/peers/add", srv.gatewayPeersAddHandler)
	handleHTTPRequest(mux, "/gateway/peers/remove", srv.gatewayPeersRemoveHandler)

	// Host API Calls
	handleHTTPRequest(mux, "/host/announce", srv.hostAnnounceHandler)
	handleHTTPRequest(mux, "/host/configure", srv.hostConfigureHandler)
	handleHTTPRequest(mux, "/host/status", srv.hostStatusHandler)

	// HostDB API Calls
	handleHTTPRequest(mux, "/hostdb/hosts/active", srv.hostdbHostsActiveHandler)
	handleHTTPRequest(mux, "/hostdb/hosts/all", srv.hostdbHostsAllHandler)

	// Miner API Calls
	handleHTTPRequest(mux, "/miner/start", srv.minerStartHandler)
	handleHTTPRequest(mux, "/miner/status", srv.minerStatusHandler)
	handleHTTPRequest(mux, "/miner/stop", srv.minerStopHandler)
	handleHTTPRequest(mux, "/miner/blockforwork", srv.minerBlockforworkHandler)
	handleHTTPRequest(mux, "/miner/submitblock", srv.minerSubmitBlockHandler)

	// Renter API Calls
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
	handleHTTPRequest(mux, "/renter/status", srv.renterStatusHandler) // TODO: alter

	// TransactionPool API Calls
	handleHTTPRequest(mux, "/transactionpool/transactions", srv.transactionpoolTransactionsHandler)

	// Wallet API Calls
	handleHTTPRequest(mux, "/wallet/address", srv.walletAddressHandler)
	handleHTTPRequest(mux, "/wallet/send", srv.walletSendHandler)
	handleHTTPRequest(mux, "/wallet/status", srv.walletStatusHandler)

	// create graceful HTTP server
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

// Serve listens for and handles API calls. It a blocking function.
func (srv *Server) Serve() error {
	// graceful will run until it catches a signal.
	// It can also be stopped manually by stopHandler.
	err := srv.apiServer.ListenAndServe()
	// despite its name, graceful still propogates this benign error
	if err != nil && strings.HasSuffix(err.Error(), "use of closed network connection") {
		err = nil
	}
	fmt.Println("\rCaught stop signal, quitting.")
	return err
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
