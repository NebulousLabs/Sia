package main

import (
	"encoding/json"
	"net/http"
	"time"

	//TODO: remove graceful dependencies?
	"github.com/stretchr/graceful"
)

// TODO: timeouts?
func (d *daemon) handle(addr string) (err error) {
	handler := http.NewServeMux()

	// Web Interface
	handler.HandleFunc("/", d.webIndex)
	handler.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir(d.styleDir))))

	// Host API Calls
	//
	// TODO: SetConfig also calls announce(), there should be smarter ways to
	// handle this.
	handler.HandleFunc("/host/config", d.hostConfigHandler)
	handler.HandleFunc("/host/setconfig", d.hostSetConfigHandler)

	// Miner API Calls
	handler.HandleFunc("/miner/start", d.minerStartHandler)
	handler.HandleFunc("/miner/status", d.minerStatusHandler)
	handler.HandleFunc("/miner/stop", d.minerStopHandler)

	// Wallet API Calls
	handler.HandleFunc("/wallet/address", d.walletAddressHandler)
	handler.HandleFunc("/wallet/send", d.walletSendHandler)
	handler.HandleFunc("/wallet/status", d.walletStatusHandler)

	// File API Calls
	handler.HandleFunc("/file/upload", d.fileUploadHandler)
	handler.HandleFunc("/file/uploadpath", d.fileUploadPathHandler)
	handler.HandleFunc("/file/download", d.fileDownloadHandler)
	handler.HandleFunc("/file/status", d.fileStatusHandler)

	// Peer API Calls
	handler.HandleFunc("/peer/add", d.peerAddHandler)
	handler.HandleFunc("/peer/remove", d.peerRemoveHandler)
	handler.HandleFunc("/peer/status", d.peerStatusHandler)

	// Misc. API Calls
	handler.HandleFunc("/update/check", d.updateCheckHandler)
	handler.HandleFunc("/update/apply", d.updateApplyHandler)
	handler.HandleFunc("/status", d.statusHandler)
	handler.HandleFunc("/stop", d.stopHandler)
	handler.HandleFunc("/sync", d.syncHandler)
	// For debugging purposes only
	handler.HandleFunc("/mutextest", d.mutexTestHandler)

	server := &graceful.Server{
		Server: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}

	println("P1")
	go server.ListenAndServe()
	println("P2")
	<-d.stop
	println("P3")
	d.core.Close()
	println("P4")
	server.Stop(10 * time.Millisecond)
	println("P5")

	return nil
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
