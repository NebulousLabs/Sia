package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stretchr/graceful"
)

const (
	apiTimeout = 5 * time.Second
)

// handleHTTPRequest is a wrapper function that logs and then handles all
// incoming calls to the API.
func handleHTTPRequest(mux *http.ServeMux, url string, handler http.HandlerFunc) {
	mux.HandleFunc(url, func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s", req.Method, req.URL)
		handler(w, req)
	})
}

// initAPI determines which functions handle each API call.
func (srv *Server) initAPI(addr string) {
	mux := http.NewServeMux()

	// Consensus API Calls
	handleHTTPRequest(mux, "/consensus/status", srv.consensusStatusHandler)
	handleHTTPRequest(mux, "/consensus/synchronize", srv.consensusSynchronizeHandler)

	// Server API Calls
	handleHTTPRequest(mux, "/daemon/stop", srv.daemonStopHandler)
	handleHTTPRequest(mux, "/daemon/update/apply", srv.daemonUpdateApplyHandler)
	handleHTTPRequest(mux, "/daemon/update/check", srv.daemonUpdateCheckHandler)

	// Debugging API Calls
	handleHTTPRequest(mux, "/debug/constants", srv.debugConstantsHandler)
	handleHTTPRequest(mux, "/debug/mutextest", srv.mutexTestHandler)

	// Gateway API Calls
	handleHTTPRequest(mux, "/gateway/status", srv.gatewayStatusHandler)
	handleHTTPRequest(mux, "/gateway/peer/add", srv.gatewayPeerAddHandler)
	handleHTTPRequest(mux, "/gateway/peer/remove", srv.gatewayPeerRemoveHandler)

	// Host API Calls
	handleHTTPRequest(mux, "/host/announce", srv.hostAnnounceHandler)
	handleHTTPRequest(mux, "/host/config", srv.hostConfigHandler)
	handleHTTPRequest(mux, "/host/status", srv.hostStatusHandler)

	// HostDB API Calls
	handleHTTPRequest(mux, "/hostdb/hosts/active", srv.hostdbHostsActiveHandler)
	handleHTTPRequest(mux, "/hostdb/hosts/all", srv.hostdbHostsAllHandler)

	// Miner API Calls
	handleHTTPRequest(mux, "/miner/start", srv.minerStartHandler)
	handleHTTPRequest(mux, "/miner/status", srv.minerStatusHandler)
	handleHTTPRequest(mux, "/miner/stop", srv.minerStopHandler)

	// Renter API Calls
	handleHTTPRequest(mux, "/renter/download", srv.renterDownloadHandler)
	handleHTTPRequest(mux, "/renter/downloadqueue", srv.renterDownloadqueueHandler)
	handleHTTPRequest(mux, "/renter/files", srv.renterFilesHandler)
	handleHTTPRequest(mux, "/renter/status", srv.renterStatusHandler)
	handleHTTPRequest(mux, "/renter/upload", srv.renterUploadHandler)

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

// Serve listens for and handles API calls. It a blocking function.
func (srv *Server) Serve() error {
	// graceful will run until it catches a signal.
	// It can also be stopped manually by stopHandler.
	err := srv.apiServer.ListenAndServe()
	// despite its name, graceful still propogates this benign error
	if err != nil && strings.HasSuffix(err.Error(), "use of closed network connection") {
		err = nil
	}
	println("\rCaught stop signal, quitting.")
	return err
}

// writeError logs an writes an error to the API caller.
func writeError(w http.ResponseWriter, msg string, err int) {
	log.Printf("%d HTTP ERROR: %s", err, msg)
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
