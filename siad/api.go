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

	// Host API Calls
	handleHTTPRequest(mux, "/host/config", d.hostConfigHandler)
	handleHTTPRequest(mux, "/host/announce", d.hostAnnounceHandler)
	handleHTTPRequest(mux, "/host/status", d.hostStatusHandler)

	// Miner API Calls
	handleHTTPRequest(mux, "/miner/start", d.minerStartHandler)
	handleHTTPRequest(mux, "/miner/status", d.minerStatusHandler)
	handleHTTPRequest(mux, "/miner/stop", d.minerStopHandler)

	// Wallet API Calls
	handleHTTPRequest(mux, "/wallet/address", d.walletAddressHandler)
	handleHTTPRequest(mux, "/wallet/send", d.walletSendHandler)
	handleHTTPRequest(mux, "/wallet/status", d.walletStatusHandler)

	// File API Calls
	handleHTTPRequest(mux, "/file/upload", d.fileUploadHandler)
	handleHTTPRequest(mux, "/file/uploadpath", d.fileUploadPathHandler)
	handleHTTPRequest(mux, "/file/download", d.fileDownloadHandler)
	handleHTTPRequest(mux, "/file/status", d.fileStatusHandler)

	// Peer API Calls
	handleHTTPRequest(mux, "/peer/add", d.peerAddHandler)
	handleHTTPRequest(mux, "/peer/remove", d.peerRemoveHandler)
	handleHTTPRequest(mux, "/peer/status", d.peerStatusHandler)

	// Misc. API Calls
	handleHTTPRequest(mux, "/update/check", d.updateCheckHandler)
	handleHTTPRequest(mux, "/update/apply", d.updateApplyHandler)
	handleHTTPRequest(mux, "/status", d.statusHandler)
	handleHTTPRequest(mux, "/stop", d.stopHandler)
	handleHTTPRequest(mux, "/sync", d.syncHandler)

	// Debugging API Calls
	handleHTTPRequest(mux, "/debug/constants", d.debugConstantsHandler)
	handleHTTPRequest(mux, "/debug/mutextest", d.mutexTestHandler)

	handleHTTPRequest(mux, "/mutextest", d.mutexTestHandler)

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
