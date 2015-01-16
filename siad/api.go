package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/NebulousLabs/Sia/network"
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
	http.HandleFunc("file/upload", d.fileUploadHandler)
	http.HandleFunc("file/download", d.fileDownloadHandler)
	http.HandleFunc("file/status", d.fileStatusHandler)

	// Misc. API Calls
	http.HandleFunc("/sync", d.syncHandler)
	http.HandleFunc("/peer/add", d.peerAddHandler)
	http.HandleFunc("/peer/remove", d.peerRemoveHandler)
	http.HandleFunc("/status", d.statusHandler)
	http.HandleFunc("/update/check", d.updateCheckHandler)
	http.HandleFunc("/update/apply", d.updateApplyHandler)
	http.HandleFunc("/stop", d.stopHandler)

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

func (d *daemon) statusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.core.StateInfo())
}

func (d *daemon) stopHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: more graceful shutdown?
	d.core.Close()
	os.Exit(0)
}

func (d *daemon) syncHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: don't spawn multiple CatchUps
	if len(d.core.AddressBook()) == 0 {
		http.Error(w, "No peers available for syncing", 500)
		return
	}

	go d.core.CatchUp(d.core.RandomPeer())

	writeSuccess(w)
}

func (d *daemon) peerAddHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: this should return an error
	d.core.AddPeer(network.Address(req.FormValue("addr")))

	writeSuccess(w)
}

func (d *daemon) peerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: this should return an error
	d.core.RemovePeer(network.Address(req.FormValue("addr")))

	writeSuccess(w)
}

func (d *daemon) updateCheckHandler(w http.ResponseWriter, req *http.Request) {
	available, version, err := d.checkForUpdate()
	if err != nil {
		http.Error(w, err.Error(), 500)
	} else {
		writeJSON(w, struct {
			Available bool
			Version   string
		}{available, version})
	}
}

func (d *daemon) updateApplyHandler(w http.ResponseWriter, req *http.Request) {
	err := d.applyUpdate(req.FormValue("version"))
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	writeSuccess(w)
}
