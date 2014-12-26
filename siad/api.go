package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/sia"
)

// TODO: timeouts?
func (d *daemon) setUpHandlers(addr string) {
	// Web Interface
	http.HandleFunc("/", d.webIndex)
	http.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir(d.styleDir))))

	// Plaintext API
	http.HandleFunc("/sync", d.syncHandler)
	http.HandleFunc("/peer", d.peerHandler)
	http.HandleFunc("/host", d.hostHandler)
	http.HandleFunc("/rent", d.rentHandler)
	http.HandleFunc("/download", d.downloadHandler)
	http.HandleFunc("/status", d.statusHandler)
	http.HandleFunc("/stop", d.stopHandler)

	// Wallet API Calls
	http.HandleFunc("/wallet/address", d.walletAddressHandler)
	http.HandleFunc("/wallet/send", d.walletSendHandler)
	http.HandleFunc("/wallet/status", d.walletStatusHandler)

	// Miner API Calls
	http.HandleFunc("/miner/start", d.minerStartHandler)
	http.HandleFunc("/miner/status", d.minerStatusHandler)
	http.HandleFunc("/miner/stop", d.minerStopHandler)

	// JSON API
	http.HandleFunc("/json/status", d.jsonStatusHandler)

	http.ListenAndServe(addr, nil)
}

// jsonStatusHandler responds to a status call with a json object of the status.
func (d *daemon) jsonStatusHandler(w http.ResponseWriter, req *http.Request) {
	status := d.core.Info()
	resp, err := json.Marshal(status)
	if err != nil {
		http.Error(w, "Failed to encode status object", 500)
		return
	}
	w.Write(resp)
}

func (d *daemon) stopHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: more graceful shutdown?
	d.core.Close()
	os.Exit(0)
}

func (d *daemon) syncHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: don't spawn multiple CatchUps
	// TODO: return error if no peers exist
	go d.core.CatchUp(d.core.RandomPeer())
	fmt.Fprint(w, "Sync initiated")
}

func (d *daemon) peerHandler(w http.ResponseWriter, req *http.Request) {
	addr := network.Address(req.FormValue("addr"))
	switch req.FormValue("action") {
	case "add":
		d.core.AddPeer(addr)
		fmt.Fprintf(w, "Added %s", req.FormValue("addr"))
	case "remove":
		d.core.RemovePeer(addr)
		fmt.Fprintf(w, "Removed %s", req.FormValue("addr"))
	default:
		http.Error(w, "Invalid peer action", 400)
	}
}

func (d *daemon) hostHandler(w http.ResponseWriter, req *http.Request) {
	// Create all of the variables that get scanned in.
	var ipAddress network.Address
	var totalStorage int64
	var minFilesize, maxFilesize, minTolerance uint64
	var minDuration, maxDuration, minWindow, maxWindow, freezeDuration consensus.BlockHeight
	var price, burn, freezeCoins consensus.Currency
	var coinAddress consensus.CoinAddress

	// Get the ip address.
	ipAddress = network.Address(req.FormValue("ipaddress"))

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

	// Make the host announcement.
	_, err := d.core.HostAnnounceSelf(freezeCoins, freezeDuration+d.core.Height(), 10)
	if err != nil {
		http.Error(w, "Failed to announce host: "+err.Error(), 500)
		return
	}

	fmt.Fprint(w, "Update successful")
}

func (d *daemon) rentHandler(w http.ResponseWriter, req *http.Request) {
	filename, nickname := req.FormValue("sourcefile"), req.FormValue("nickname")
	err := d.core.ClientProposeContract(filename, nickname)
	if err != nil {
		http.Error(w, "Failed to create file contract: "+err.Error(), 500)
	} else {
		fmt.Fprintf(w, "Upload complete: %s (%s)", nickname, filename)
	}
}

func (d *daemon) downloadHandler(w http.ResponseWriter, req *http.Request) {
	nickname, filename := req.FormValue("nickname"), req.FormValue("destination")
	if filename == "" {
		filename = d.downloadDir + nickname
	}
	err := d.core.Download(nickname, filename)
	if err != nil {
		http.Error(w, "Failed to download file: "+err.Error(), 500)
	} else {
		fmt.Fprint(w, "Download complete!")
	}
}

// TODO: this should probably just return JSON. Leave formatting to the client.
func (d *daemon) statusHandler(w http.ResponseWriter, req *http.Request) {
	info := d.core.StateInfo()

	// set mining status
	mineStatus := "OFF"

	// create peer listing
	peers := "\n"
	for _, addr := range d.core.AddressBook() {
		peers += fmt.Sprintf("\t\t%s\n", addr)
	}

	// create friend listing
	friends := "\n"
	for name, address := range d.core.FriendMap() {
		friends += fmt.Sprintf("\t\t%v\t%x\n", name, address)
	}

	// write stats to ResponseWriter
	fmt.Fprintf(w, `General Information:

	Mining Status: %s

	Wallet Balance: %v
	Full Wallet Balance: %v

	Current Block Height: %v
	Current Block Target: %v
	Current Block Depth: %v

	Networked Peers: %s

	Friends: %s`,
		mineStatus, d.core.WalletBalance(false), d.core.WalletBalance(true),
		info.Height, info.Target, info.Depth, peers, friends,
	)
}
