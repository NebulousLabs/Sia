package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/stretchr/graceful"
)

const apiTimeout = 5e9 // 5 seconds

func logWrapper(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		log.Printf("%s %s", req.Method, req.URL)
		f(w, req)
	}
}

func (d *daemon) listen(addr string) {
	mux := http.NewServeMux()

	// Host API Calls
	mux.HandleFunc("/host/config", logWrapper(d.hostConfigHandler))
	mux.HandleFunc("/host/announce", logWrapper(d.hostAnnounceHandler))
	mux.HandleFunc("/host/status", logWrapper(d.hostStatusHandler))

	// Miner API Calls
	mux.HandleFunc("/miner/start", logWrapper(d.minerStartHandler))
	mux.HandleFunc("/miner/status", logWrapper(d.minerStatusHandler))
	mux.HandleFunc("/miner/stop", logWrapper(d.minerStopHandler))

	// Wallet API Calls
	mux.HandleFunc("/wallet/address", logWrapper(d.walletAddressHandler))
	mux.HandleFunc("/wallet/send", logWrapper(d.walletSendHandler))
	mux.HandleFunc("/wallet/status", logWrapper(d.walletStatusHandler))

	// File API Calls
	mux.HandleFunc("/file/upload", logWrapper(d.fileUploadHandler))
	mux.HandleFunc("/file/uploadpath", logWrapper(d.fileUploadPathHandler))
	mux.HandleFunc("/file/download", logWrapper(d.fileDownloadHandler))
	mux.HandleFunc("/file/status", logWrapper(d.fileStatusHandler))

	// Peer API Calls
	mux.HandleFunc("/peer/add", logWrapper(d.peerAddHandler))
	mux.HandleFunc("/peer/remove", logWrapper(d.peerRemoveHandler))
	mux.HandleFunc("/peer/status", logWrapper(d.peerStatusHandler))

	// Misc. API Calls
	mux.HandleFunc("/update/check", logWrapper(d.updateCheckHandler))
	mux.HandleFunc("/update/apply", logWrapper(d.updateApplyHandler))
	mux.HandleFunc("/status", logWrapper(d.statusHandler))
	mux.HandleFunc("/stop", logWrapper(d.stopHandler))
	mux.HandleFunc("/sync", logWrapper(d.syncHandler))

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
