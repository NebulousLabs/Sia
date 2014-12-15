package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

// TODO: timeouts?
func (d *daemon) setUpHandlers(apiPort uint16) {
	// Web Interface
	http.HandleFunc("/", d.webIndex)
	http.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir(d.styleDir))))

	// Plaintext API
	http.HandleFunc("/sync", d.syncHandler)
	http.HandleFunc("/mine", d.mineHandler)
	http.HandleFunc("/sendcoins", d.sendHandler)
	http.HandleFunc("/host", d.hostHandler)
	http.HandleFunc("/rent", d.rentHandler)
	http.HandleFunc("/download", d.downloadHandler)
	http.HandleFunc("/save", d.saveHandler)
	http.HandleFunc("/load", d.loadHandler)
	http.HandleFunc("/status", d.statusHandler)
	http.HandleFunc("/stop", d.stopHandler)

	// JSON API
	http.HandleFunc("/json/status", d.jsonStatusHandler)

	http.ListenAndServe("localhost:"+strconv.Itoa(int(apiPort)), nil)
}

// jsonStatusHandler responds to a status call with a json object of the status.
func (d *daemon) jsonStatusHandler(w http.ResponseWriter, req *http.Request) {
	status := d.core.EnvironmentInfo()
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

func (d *daemon) mineHandler(w http.ResponseWriter, req *http.Request) {
	switch req.FormValue("toggle") {
	case "on":
		d.core.StartMining()
		fmt.Fprint(w, "Started mining")
	case "off":
		d.core.StopMining()
		fmt.Fprint(w, "Stopped mining")
	default:
		http.Error(w, "Invalid mine command", 400)
	}
}

func (d *daemon) sendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the inputs.
	var amount, fee consensus.Currency
	var dest consensus.CoinAddress
	_, err := fmt.Sscan(req.FormValue("amount"), &amount)
	if err != nil {
		http.Error(w, "Malformed amount", 400)
		return
	}
	_, err = fmt.Sscan(req.FormValue("fee"), &fee)
	if err != nil {
		http.Error(w, "Malformed fee", 400)
		return
	}

	// dest can be either a coin address or a friend name
	destString := req.FormValue("dest")
	// if ca, ok := e.friends[destString]; ok {
	//	dest = ca
	//} else
	if len(destString) != 64 {
		http.Error(w, "Friend not found (or malformed coin address)", 400)
		return
	} else {
		var destAddressBytes []byte
		_, err = fmt.Sscanf(destString, "%x", &destAddressBytes)
		if err != nil {
			http.Error(w, "Malformed coin address", 400)
			return
		}
		copy(dest[:], destAddressBytes)
	}

	// Spend the coins.
	_, err = d.core.SpendCoins(amount, fee, dest)
	if err != nil {
		http.Error(w, "Failed to create transaction: "+err.Error(), 500)
		return
	}

	fmt.Fprintf(w, "Sent %v coins to %x, with fee of %v", amount, dest, fee)
}

func (d *daemon) hostHandler(w http.ResponseWriter, req *http.Request) {
	// Create all of the variables that get scanned in.
	var ipAddress network.NetAddress
	var totalStorage int64
	var minFilesize, maxFilesize, minTolerance uint64
	var minDuration, maxDuration, minWindow, maxWindow, freezeDuration consensus.BlockHeight
	var price, burn, freezeCoins consensus.Currency
	var coinAddress consensus.CoinAddress

	// Get the ip address.
	hostAndPort := strings.Split(req.FormValue("ipaddress"), ":")
	if len(hostAndPort) != 2 {
		http.Error(w, "Malformed IP address + port", 400)
		return
	}
	_, err := fmt.Sscan(hostAndPort[0], &ipAddress.Host)
	if err != nil {
		http.Error(w, "Malformed IP address", 400)
		return
	}
	_, err = fmt.Sscan(hostAndPort[1], &ipAddress.Port)
	if err != nil {
		http.Error(w, "Malformed port number", 400)
		return
	}

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
		_, err = fmt.Sscanf(caString, "%x", &coinAddressBytes)
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
		_, err = fmt.Sscan(req.FormValue(qs), qsVars[qs])
		if err != nil {
			http.Error(w, "Malformed "+qs, 400)
			return
		}
	}

	// Set the host settings.
	d.core.SetHostSettings(siacore.HostAnnouncement{
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
	_, err = d.core.HostAnnounceSelf(freezeCoins, freezeDuration+d.core.Height(), 10)
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

func (d *daemon) saveHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename := req.FormValue("filename")
	err := d.core.SaveCoinAddress(filename)
	if err != nil {
		http.Error(w, "Failed to save coin address: "+err.Error(), 500)
	} else {
		fmt.Fprint(w, "Saved coin address to "+filename)
	}
}

func (d *daemon) loadHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename, friendname := req.FormValue("filename"), req.FormValue("friendname")
	err := d.core.LoadCoinAddress(filename, friendname)
	if err != nil {
		http.Error(w, "Failed to load coin address: "+err.Error(), 500)
	} else {
		fmt.Fprint(w, "Loaded coin address from "+filename)
	}
}

// TODO: this should probably just return JSON. Leave formatting to the client.
func (d *daemon) statusHandler(w http.ResponseWriter, req *http.Request) {
	// get state info
	info := d.core.StateInfo()
	// set mining status
	mineStatus := "OFF"
	if d.core.Mining() {
		mineStatus = "ON"
	}
	// create peer listing
	peers := "\n"
	for _, address := range d.core.AddressBook() {
		peers += fmt.Sprintf("\t\t%v:%v\n", address.Host, address.Port)
	}
	// create friend listing
	friends := "\n"
	for name, address := range d.core.FriendMap() {
		friends += fmt.Sprintf("\t\t%v\t%x\n", name, address)
	}
	// write stats to ResponseWriter
	fmt.Fprintf(w, `General Information:

	Mining Status: %s

	Wallet Address: %x
	Wallet Balance: %v

	Current Block Height: %v
	Current Block Target: %v
	Current Block Depth: %v

	Networked Peers: %s

	Friends: %s`,
		mineStatus, d.core.CoinAddress(), d.core.WalletBalance(),
		info.Height, info.Target, info.Depth, peers, friends,
	)
}
