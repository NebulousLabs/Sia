package main

import (
	"encoding/json"
	"net/http"

	"github.com/stretchr/graceful"
)

const apiTimeout = 5e9 // 5 seconds

// TODO: timeouts?
func (d *daemon) handle(addr string) {
	mux := http.NewServeMux()

	// Web Interface
	mux.HandleFunc("/", d.webIndex)
	mux.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir(d.styleDir))))

	// Host API Calls
	//
	// TODO: SetConfig also calls announce(), there should be smarter ways to
	// handle this.
	mux.HandleFunc("/host/config", d.hostConfigHandler)
	mux.HandleFunc("/host/setconfig", d.hostSetConfigHandler)

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
	mux.HandleFunc("/mutextest", d.mutexTestHandler)

	// create graceful HTTP server
	d.apiServer = &graceful.Server{
		Timeout: apiTimeout,
		Server:  &http.Server{Addr: addr, Handler: mux},
	}

	// graceful will run until it catches a signal.
	// it can also be stopped manually by stopHandler.
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

func (d *daemon) mutexTestHandler(w http.ResponseWriter, req *http.Request) {
	d.core.ScanMutexes()
}
