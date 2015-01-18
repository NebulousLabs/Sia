package main

import (
	"encoding/json"
	"net/http"
)

// TODO: timeouts?
func (d *daemon) handle(addr string) {
	// Web Interface
	http.HandleFunc("/", d.webIndex)
	http.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir(d.styleDir))))

	// Host API Calls
	//
	// TODO: SetConfig also calls announce(), there should be smarter ways to
	// handle this.
	http.HandleFunc("/host/config", d.hostConfigHandler)
	http.HandleFunc("/host/setconfig", d.hostSetConfigHandler)

	// Miner API Calls
	http.HandleFunc("/miner/start", d.minerStartHandler)
	http.HandleFunc("/miner/status", d.minerStatusHandler)
	http.HandleFunc("/miner/stop", d.minerStopHandler)

	// Wallet API Calls
	http.HandleFunc("/wallet/address", d.walletAddressHandler)
	http.HandleFunc("/wallet/send", d.walletSendHandler)
	http.HandleFunc("/wallet/status", d.walletStatusHandler)

	// File API Calls
	http.HandleFunc("/file/upload", d.fileUploadHandler)
	http.HandleFunc("/file/download", d.fileDownloadHandler)
	http.HandleFunc("/file/status", d.fileStatusHandler)

	// Peer API Calls
	http.HandleFunc("/peer/add", d.peerAddHandler)
	http.HandleFunc("/peer/remove", d.peerRemoveHandler)
	http.HandleFunc("/peer/status", d.peerStatusHandler)

	// Misc. API Calls
	http.HandleFunc("/sync", d.syncHandler)
	http.HandleFunc("/status", d.statusHandler)
	http.HandleFunc("/update/check", d.updateCheckHandler)
	http.HandleFunc("/update/apply", d.updateApplyHandler)
	http.HandleFunc("/stop", d.stopHandler)
	http.HandleFunc("/mutextest", d.mutexTestHandler)

	http.ListenAndServe(addr, nil)
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
