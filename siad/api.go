package main

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
// incoming calls to the api.
func handleHTTPRequest(mux *http.ServeMux, url string, handler http.HandlerFunc) {
	mux.HandleFunc(url, func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s", req.Method, req.URL)
		handler(w, req)
	})
}

// initAPI determines which functions handle each api call.
func (d *daemon) initAPI(addr string) {
	mux := http.NewServeMux()

	// Consensus API Calls
	handleHTTPRequest(mux, "/consensus/status", d.consensusStatusHandler)

	// Daemon API Calls
	handleHTTPRequest(mux, "/daemon/stop", d.daemonStopHandler)
	handleHTTPRequest(mux, "/daemon/update/apply", d.daemonUpdateApplyHandler)
	handleHTTPRequest(mux, "/daemon/update/check", d.daemonUpdateCheckHandler)

	// Debugging API Calls
	handleHTTPRequest(mux, "/debug/constants", d.debugConstantsHandler)
	handleHTTPRequest(mux, "/debug/mutextest", d.mutexTestHandler)

	// Gateway API Calls
	handleHTTPRequest(mux, "/gateway/status", d.gatewayStatusHandler)
	handleHTTPRequest(mux, "/gateway/synchronize", d.gatewaySynchronizeHandler)
	handleHTTPRequest(mux, "/gateway/peer/add", d.gatewayPeerAddHandler)
	handleHTTPRequest(mux, "/gateway/peer/remove", d.gatewayPeerRemoveHandler)

	// Host API Calls
	handleHTTPRequest(mux, "/host/announce", d.hostAnnounceHandler)
	handleHTTPRequest(mux, "/host/config", d.hostConfigHandler)
	handleHTTPRequest(mux, "/host/status", d.hostStatusHandler)

	// HostDB API Calls

	// Miner API Calls
	handleHTTPRequest(mux, "/miner/start", d.minerStartHandler)
	handleHTTPRequest(mux, "/miner/status", d.minerStatusHandler)
	handleHTTPRequest(mux, "/miner/stop", d.minerStopHandler)

	// Renter API Calls
	handleHTTPRequest(mux, "/renter/download", d.renterDownloadHandler)
	handleHTTPRequest(mux, "/renter/status", d.renterStatusHandler)
	handleHTTPRequest(mux, "/renter/upload", d.renterUploadHandler)

	// TransactionPool API Calls

	// Wallet API Calls
	handleHTTPRequest(mux, "/wallet/address", d.walletAddressHandler)
	handleHTTPRequest(mux, "/wallet/send", d.walletSendHandler)
	handleHTTPRequest(mux, "/wallet/status", d.walletStatusHandler)

	// create graceful HTTP server
	d.apiServer = &graceful.Server{
		Timeout: apiTimeout,
		Server:  &http.Server{Addr: addr, Handler: mux},
	}
}

// listen starts listening on the port for api calls.
func (d *daemon) listen() error {
	// graceful will run until it catches a signal.
	// It can also be stopped manually by stopHandler.
	err := d.apiServer.ListenAndServe()
	// despite its name, graceful still propogates this benign error
	if err != nil && strings.HasSuffix(err.Error(), "use of closed network connection") {
		err = nil
	}
	return err
}

// writeError logs an writes an error to the api caller.
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
	writeJSON(w, struct {
		Success bool
	}{true})
}
