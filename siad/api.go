package main

import (
	"encoding/json"
	"net/http"

	"github.com/stretchr/graceful"
)

const apiTimeout = 5e9 // 5 seconds

func (d *daemon) listen(addr string) {
	mux := http.NewServeMux()

	// Host API Calls
	mux.HandleFunc("/host/config", d.hostConfigHandler)
	mux.HandleFunc("/host/announce", d.hostAnnounceHandler)
	mux.HandleFunc("/host/status", d.hostStatusHandler)

	// Miner API Calls
	mux.HandleFunc("/miner/start", d.minerStartHandler)
	mux.HandleFunc("/miner/status", d.minerStatusHandler)
	mux.HandleFunc("/miner/stop", d.minerStopHandler)

	// Wallet API Calls
	mux.HandleFunc("/wallet/address", d.walletAddressHandler)
	mux.HandleFunc("/wallet/send", d.walletSendHandler)
	mux.HandleFunc("/wallet/status", d.walletStatusHandler)

	// File API Calls
	mux.HandleFunc("/file/upload", d.fileUploadHandler)
	mux.HandleFunc("/file/uploadpath", d.fileUploadPathHandler)
	mux.HandleFunc("/file/download", d.fileDownloadHandler)
	mux.HandleFunc("/file/status", d.fileStatusHandler)

	// Peer API Calls
	mux.HandleFunc("/peer/add", d.peerAddHandler)
	mux.HandleFunc("/peer/remove", d.peerRemoveHandler)
	mux.HandleFunc("/peer/status", d.peerStatusHandler)

	// Misc. API Calls
	mux.HandleFunc("/update/check", d.updateCheckHandler)
	mux.HandleFunc("/update/apply", d.updateApplyHandler)
	mux.HandleFunc("/status", d.statusHandler)
	mux.HandleFunc("/stop", d.stopHandler)
	mux.HandleFunc("/sync", d.syncHandler)

	// Debugging API Calls
	mux.HandleFunc("/debug/constants", d.debugConstantsHandler)
	mux.HandleFunc("/debug/mutextest", d.mutexTestHandler)

	// create graceful HTTP server
	d.apiServer = &graceful.Server{
		Timeout: apiTimeout,
		Server:  &http.Server{Addr: addr, Handler: mux},
	}

	// graceful will run until it catches a signal.
	// it can also be stopped manually by stopHandler.
	//
	// TODO: this fails silently. The error should be checked, but then it
	// will print an error even if interrupted normally. Need a better
	// solution.
	d.apiServer.ListenAndServe()
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
