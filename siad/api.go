package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	// "github.com/NebulousLabs/Sia/sia"
)

// TODO: timeouts?
func (d *daemon) handle(addr string) {
	// Web Interface
	http.HandleFunc("/", d.webIndex)
	http.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir(d.styleDir))))

	// Wallet API Calls
	http.HandleFunc("/wallet/address", d.walletAddressHandler)
	http.HandleFunc("/wallet/send", d.walletSendHandler)
	http.HandleFunc("/wallet/status", d.walletStatusHandler)

	// Miner API Calls
	http.HandleFunc("/miner/start", d.minerStartHandler)
	http.HandleFunc("/miner/status", d.minerStatusHandler)
	http.HandleFunc("/miner/stop", d.minerStopHandler)

	// File API Calls
	http.HandleFunc("/host", d.hostHandler)
	http.HandleFunc("/rent", d.rentHandler)
	http.HandleFunc("/download", d.downloadHandler)

	// Misc. API Calls
	http.HandleFunc("/sync", d.syncHandler)
	http.HandleFunc("/peer", d.peerHandler)
	http.HandleFunc("/status", d.statusHandler)
	http.HandleFunc("/update", d.updateHandler)
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

// success wraps a boolean in a struct for easier JSON parsing
type success struct {
	Success bool
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
	havePeers := len(d.core.AddressBook()) == 0
	if havePeers {
		go d.core.CatchUp(d.core.RandomPeer())
	}
	writeJSON(w, success{havePeers})
}

func (d *daemon) peerHandler(w http.ResponseWriter, req *http.Request) {
	addr := network.Address(req.FormValue("addr"))
	switch req.FormValue("action") {
	case "add":
		d.core.AddPeer(addr)
	case "remove":
		d.core.RemovePeer(addr)
	default:
		http.Error(w, "Invalid peer action", 400)
		return
	}
	// TODO: should Add/RemovePeer return a bool?
	writeJSON(w, success{true})
}

func (d *daemon) hostHandler(w http.ResponseWriter, req *http.Request) {
	// Create all of the variables that get scanned in.
	// var ipAddress network.Address
	var totalStorage int64
	var minFilesize, maxFilesize, minTolerance uint64
	var minDuration, maxDuration, minWindow, maxWindow, freezeDuration consensus.BlockHeight
	var price, burn, freezeCoins consensus.Currency
	var coinAddress consensus.CoinAddress

	// Get the ip address.
	// ipAddress = network.Address(req.FormValue("ipaddress"))

	// The address can be either a coin address or a friend name
	caString := req.FormValue("coinaddress")
	// if ca, ok := e.friends[caString]; ok {
	//	coinAddress = ca
	// } else
	if len(caString) != 64 {
		http.Error(w, "Friend not found (or malformed coin address)", 400)
		return
	} else {
		var coinAddressBytes []byte
		_, err := fmt.Sscanf(caString, "%x", &coinAddressBytes)
		if err != nil {
			http.Error(w, "Malformed coin address", 400)
			return
		}
		copy(coinAddress[:], coinAddressBytes)
	}

	// other vars require no special parsing
	qsVars := map[string]interface{}{
		"totalstorage":   &totalStorage,
		"minfile":        &minFilesize,
		"maxfile":        &maxFilesize,
		"mintolerance":   &minTolerance,
		"minduration":    &minDuration,
		"maxduration":    &maxDuration,
		"minwin":         &minWindow,
		"maxwin":         &maxWindow,
		"freezeduration": &freezeDuration,
		"price":          &price,
		"penalty":        &burn,
		"freezevolume":   &freezeCoins,
	}
	for qs := range qsVars {
		_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
		if err != nil {
			http.Error(w, "Malformed "+qs, 400)
			return
		}
	}

	// Set the host settings.
	/*
		d.core.SetHostSettings(sia.HostAnnouncement{
			IPAddress:          ipAddress,
			TotalStorage:       totalStorage,
			MinFilesize:        minFilesize,
			MaxFilesize:        maxFilesize,
			MinDuration:        minDuration,
			MaxDuration:        maxDuration,
			MinChallengeWindow: minWindow,
			MaxChallengeWindow: maxWindow,
			MinTolerance:       minTolerance,
			Price:              price,
			Burn:               burn,
			CoinAddress:        coinAddress,
			// SpendConditions and FreezeIndex handled by HostAnnounceSelf
		})
	*/

	/*
		// Make the host announcement.
		 _, err := d.core.HostAnnounceSelf(freezeCoins, freezeDuration+d.core.Height(), 10)
		if err != nil {
			http.Error(w, "Failed to announce host: "+err.Error(), 500)
			return
		}
	*/

	writeJSON(w, success{true})
}

func (d *daemon) rentHandler(w http.ResponseWriter, req *http.Request) {
	// filename, nickname := req.FormValue("sourcefile"), req.FormValue("nickname")
	// err := d.core.ClientProposeContract(filename, nickname)
	/*
		if err != nil {
			http.Error(w, "Failed to create file contract: "+err.Error(), 500)
		} else {
			fmt.Fprintf(w, "Upload complete: %s (%s)", nickname, filename)
		}
	*/
}

func (d *daemon) downloadHandler(w http.ResponseWriter, req *http.Request) {
	nickname, filename := req.FormValue("nickname"), req.FormValue("destination")
	if filename == "" {
		filename = d.downloadDir + nickname
	}
	/*
		 err := d.core.Download(nickname, filename)
		if err != nil {
			http.Error(w, "Failed to download file: "+err.Error(), 500)
		} else {
			fmt.Fprint(w, "Download complete!")
		}
	*/
}

func (d *daemon) updateHandler(w http.ResponseWriter, req *http.Request) {
	switch req.FormValue("action") {
	case "check":
		available, err := d.checkForUpdate()
		writeJSON(w, struct {
			Available bool
			Error     string
		}{available, err.Error()})

	case "apply":
		applied, err := d.applyUpdate()
		writeJSON(w, struct {
			Applied bool
			Error   string
		}{applied, err.Error()})

	default:
		http.Error(w, "Unrecognized action", 400)
		return
	}
}
