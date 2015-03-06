package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/stretchr/graceful"
)

const apiTimeout = 5e9 // 5 seconds

func writeError(w http.ResponseWriter, msg string, err int) {
	log.Printf("%d HTTP ERROR: %s", err, msg)
	http.Error(w, msg, err)
}

func handleHTTPRequest(mux *http.ServeMux, url string, handler http.HandlerFunc) {
	mux.HandleFunc(url, func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s", req.Method, req.URL)
		handler(w, req)
	})
}

func (d *daemon) initAPI(addr string) {
	mux := http.NewServeMux()

	// Daemon API Calls
	handleHTTPRequest(mux, "/daemon/stop", d.daemonStopHandler)
	handleHTTPRequest(mux, "/daemon/update/check", d.daemonUpdateCheckHandler)
	handleHTTPRequest(mux, "/daemon/update/apply", d.daemonUpdateApplyHandler)

	// Consensus API Calls
	handleHTTPRequest(mux, "/consensus/status", d.consensusStatusHandler)

	// Gateway API Calls
	handleHTTPRequest(mux, "/gateway/status", d.gatewayStatusHandler)
	handleHTTPRequest(mux, "/gateway/synchronize", d.gatewaySynchronizeHandler)
	handleHTTPRequest(mux, "/gateway/peer/add", d.gatewayPeerAddHandler)
	handleHTTPRequest(mux, "/gateway/peer/remove", d.gatewayPeerRemoveHandler)

	// Host API Calls
	handleHTTPRequest(mux, "/host/config", d.hostConfigHandler)
	handleHTTPRequest(mux, "/host/announce", d.hostAnnounceHandler)
	handleHTTPRequest(mux, "/host/status", d.hostStatusHandler)

	// HostDB API Calls

	// Miner API Calls
	handleHTTPRequest(mux, "/miner/start", d.minerStartHandler)
	handleHTTPRequest(mux, "/miner/status", d.minerStatusHandler)
	handleHTTPRequest(mux, "/miner/stop", d.minerStopHandler)

	// Renter API Calls
	handleHTTPRequest(mux, "/renter/upload", d.fileUploadHandler)
	handleHTTPRequest(mux, "/renter/uploadpath", d.fileUploadPathHandler)
	handleHTTPRequest(mux, "/renter/download", d.fileDownloadHandler)
	handleHTTPRequest(mux, "/renter/status", d.fileStatusHandler)

	// TransactionPool API Calls

	// Wallet API Calls
	handleHTTPRequest(mux, "/wallet/address", d.walletAddressHandler)
	handleHTTPRequest(mux, "/wallet/send", d.walletSendHandler)
	handleHTTPRequest(mux, "/wallet/status", d.walletStatusHandler)

	// Debugging API Calls
	handleHTTPRequest(mux, "/debug/constants", d.debugConstantsHandler)
	handleHTTPRequest(mux, "/debug/mutextest", d.mutexTestHandler)

	// create graceful HTTP server
	d.apiServer = &graceful.Server{
		Timeout: apiTimeout,
		Server:  &http.Server{Addr: addr, Handler: mux},
	}
}

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

// writeJSON writes the object to the ResponseWriter. If the encoding fails, an
// error is written instead.
func writeJSON(w http.ResponseWriter, obj interface{}) {
	if json.NewEncoder(w).Encode(obj) != nil {
		http.Error(w, "Failed to encode response", 500)
	}
}

// writeSuccess writes the success json object ({"Success":true}) to the
// ResponseWriter
func writeSuccess(w http.ResponseWriter) {
	writeJSON(w, struct {
		Success bool
	}{true})
}
