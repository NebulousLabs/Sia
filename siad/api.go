package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

// TODO: timeouts?
func (e *Environment) setUpHandlers(apiPort uint16) {
	// Web Interface
	http.HandleFunc("/", e.webIndex)
	http.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir(e.styleDir))))

	// Plaintext API
	http.HandleFunc("/sync", e.syncHandler)
	http.HandleFunc("/mine", e.mineHandler)
	http.HandleFunc("/sendcoins", e.sendHandler)
	http.HandleFunc("/host", e.hostHandler)
	http.HandleFunc("/rent", e.rentHandler)
	http.HandleFunc("/download", e.downloadHandler)
	http.HandleFunc("/save", e.saveHandler)
	http.HandleFunc("/load", e.loadHandler)
	http.HandleFunc("/status", e.statusHandler)
	http.HandleFunc("/stop", e.stopHandler)

	// JSON API
	http.HandleFunc("/json/status", e.jsonStatusHandler)

	http.ListenAndServe("localhost:"+strconv.Itoa(int(apiPort)), nil)
}

// jsonStatusHandler responds to a status call with a json object of the status.
func (e *Environment) jsonStatusHandler(w http.ResponseWriter, req *http.Request) {
	status := e.EnvironmentInfo()
	resp, err := json.Marshal(status)
	if err != nil {
		http.Error(w, "Failed to encode status object", 500)
		return
	}
	w.Write(resp)
}

func (e *Environment) stopHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: more graceful shutdown?
	e.Close()
	os.Exit(0)
}

func (e *Environment) syncHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: don't spawn multiple CatchUps
	// TODO: return error if no peers exist
	go e.CatchUp(e.RandomPeer())
	fmt.Fprint(w, "Sync initiated")
}

func (e *Environment) mineHandler(w http.ResponseWriter, req *http.Request) {
	switch req.FormValue("toggle") {
	case "on":
		e.StartMining()
		fmt.Fprint(w, "Started mining")
	case "off":
		e.StopMining()
		fmt.Fprint(w, "Stopped mining")
	default:
		http.Error(w, "Invalid mine command", 400)
	}
}

func (e *Environment) sendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the inputs.
	var amount, fee siacore.Currency
	var dest siacore.CoinAddress
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
	if ca, ok := e.friends[destString]; ok {
		dest = ca
	} else if len(destString) != 64 {
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
	_, err = e.SpendCoins(amount, fee, dest)
	if err != nil {
		http.Error(w, "Failed to create transaction: "+err.Error(), 500)
		return
	}

	fmt.Fprintf(w, "Sent %v coins to %x, with fee of %v", amount, dest, fee)
}

func (e *Environment) hostHandler(w http.ResponseWriter, req *http.Request) {
	// Create all of the variables that get scanned in.
	var ipAddress network.NetAddress
	var totalStorage int64
	var minFilesize, maxFilesize, minTolerance uint64
	var minDuration, maxDuration, minWindow, maxWindow, freezeDuration siacore.BlockHeight
	var price, burn, freezeCoins siacore.Currency
	var coinAddress siacore.CoinAddress

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
	if ca, ok := e.friends[caString]; ok {
		coinAddress = ca
	} else if len(caString) != 64 {
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
	e.SetHostSettings(HostAnnouncement{
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
	_, err = e.HostAnnounceSelf(freezeCoins, freezeDuration+e.Height(), 10)
	if err != nil {
		http.Error(w, "Failed to announce host: "+err.Error(), 500)
		return
	}

	fmt.Fprint(w, "Update successful")
}

func (e *Environment) rentHandler(w http.ResponseWriter, req *http.Request) {
	filename, nickname := req.FormValue("sourcefile"), req.FormValue("nickname")
	err := e.ClientProposeContract(filename, nickname)
	if err != nil {
		http.Error(w, "Failed to create file contract: "+err.Error(), 500)
	} else {
		fmt.Fprintf(w, "Upload complete: %s (%s)", nickname, filename)
	}
}

func (e *Environment) downloadHandler(w http.ResponseWriter, req *http.Request) {
	nickname, filename := req.FormValue("nickname"), req.FormValue("destination")
	if filename == "" {
		filename = e.downloadDir + nickname
	}
	err := e.Download(nickname, filename)
	if err != nil {
		http.Error(w, "Failed to download file: "+err.Error(), 500)
	} else {
		fmt.Fprint(w, "Download complete!")
	}
}

func (e *Environment) saveHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename := req.FormValue("filename")
	err := e.SaveCoinAddress(filename)
	if err != nil {
		http.Error(w, "Failed to save coin address: "+err.Error(), 500)
	} else {
		fmt.Fprint(w, "Saved coin address to "+filename)
	}
}

func (e *Environment) loadHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename, friendname := req.FormValue("filename"), req.FormValue("friendname")
	err := e.LoadCoinAddress(filename, friendname)
	if err != nil {
		http.Error(w, "Failed to load coin address: "+err.Error(), 500)
	} else {
		fmt.Fprint(w, "Loaded coin address from "+filename)
	}
}

// TODO: this should probably just return JSON. Leave formatting to the client.
func (e *Environment) statusHandler(w http.ResponseWriter, req *http.Request) {
	// get state info
	info := e.StateInfo()
	// set mining status
	mineStatus := "OFF"
	if e.Mining() {
		mineStatus = "ON"
	}
	// create peer listing
	peers := "\n"
	for _, address := range e.AddressBook() {
		peers += fmt.Sprintf("\t\t%v:%v\n", address.Host, address.Port)
	}
	// create friend listing
	friends := "\n"
	for name, address := range e.FriendMap() {
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
		mineStatus, e.CoinAddress(), e.WalletBalance(),
		info.Height, info.Target, info.Depth, peers, friends,
	)
}
