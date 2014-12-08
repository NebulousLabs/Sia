package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/NebulousLabs/Andromeda/siacore"
)

// TODO: timeouts?
func (e *Environment) setUpHandlers(apiPort uint16) {
	// Web Interface
	http.HandleFunc("/", e.webIndex)
	http.Handle("/lib/", http.StripPrefix("/lib/", http.FileServer(http.Dir("webpages"))))

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

	stringPort := string(append([]byte(":"), strconv.Itoa(int(apiPort))...)) // there's gotta be a better way to do this
	http.ListenAndServe(stringPort, nil)
}

// jsonStatusHandler responds to a status call with a json object of the status.
func (e *Environment) jsonStatusHandler(w http.ResponseWriter, req *http.Request) {
	status := e.EnvironmentInfo()
	resp, err := json.Marshal(status)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Fprintf(w, "%s", resp)
}

func (e *Environment) stopHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: more graceful shutdown?
	e.Close()
	os.Exit(0)
}

func (e *Environment) syncHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: don't spawn multiple CatchUps
	go e.CatchUp(e.RandomPeer())
	fmt.Fprint(w, "Sync initiated")
}

func (e *Environment) mineHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: start/stop subcommands
	e.ToggleMining()
	if e.Mining() {
		fmt.Fprint(w, "Started mining")
	} else {
		fmt.Fprint(w, "Stopped mining")
	}
}

func (e *Environment) sendHandler(w http.ResponseWriter, req *http.Request) {
	// Scan the inputs.
	var amount, fee siacore.Currency
	var destBytes []byte
	var dest siacore.CoinAddress
	_, err := fmt.Sscan(req.FormValue("amount"), &amount)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}
	_, err = fmt.Sscan(req.FormValue("fee"), &fee)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}
	_, err = fmt.Sscanf(req.FormValue("dest"), "%x", &destBytes)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}

	// Sanity check the address.
	// TODO: Make addresses checksummed or reed-solomon encoded.
	if len(destBytes) != len(dest) {
		fmt.Fprint(w, "address is not sufficiently long")
		return
	}
	copy(dest[:], destBytes)

	// Spend the coins.
	_, err = e.SpendCoins(amount, fee, dest)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}

	fmt.Fprintf(w, "Sent %v coins to %x, with fee of %v", amount, dest, fee)
}

func (e *Environment) hostHandler(w http.ResponseWriter, req *http.Request) {
	var MB uint64
	var price, freezeCoins siacore.Currency
	var freezeBlocks siacore.BlockHeight
	// scan values
	// TODO: check error
	fmt.Sscan(req.FormValue("MB"), &MB)
	fmt.Sscan(req.FormValue("price"), &price)
	fmt.Sscan(req.FormValue("freezeCoins"), &freezeCoins)
	fmt.Sscan(req.FormValue("freezeBlocks"), &freezeBlocks)

	e.SetHostSettings(HostAnnouncement{
		IPAddress:             e.NetAddress(),
		MinFilesize:           1024 * 1024, // 1 MB
		MaxFilesize:           MB * 1024 * 1024,
		MinDuration:           2000,
		MaxDuration:           10000,
		MinChallengeFrequency: 250,
		MaxChallengeFrequency: 100,
		MinTolerance:          10,
		Price:                 price,
		Burn:                  price,
		CoinAddress:           e.CoinAddress(),
		// SpendConditions and FreezeIndex handled by HostAnnounceSelf
	})
	_, err := e.HostAnnounceSelf(freezeCoins, freezeBlocks+e.Height(), 10)
	if err != nil {
		fmt.Fprint(w, err)
	} else {
		fmt.Fprint(w, "Announce successful")
	}
}

func (e *Environment) rentHandler(w http.ResponseWriter, req *http.Request) {
	filename := req.FormValue("filename")
	err := e.ClientProposeContract(filename)
	if err != nil {
		fmt.Fprint(w, err)
	} else {
		fmt.Fprint(w, "Upload complete: "+filename)
	}
}

func (e *Environment) downloadHandler(w http.ResponseWriter, req *http.Request) {
	filename := req.FormValue("filename")
	err := e.Download(filename)
	if err != nil {
		fmt.Fprint(w, err)
	} else {
		fmt.Fprint(w, "Download complete: "+filename)
	}
}

func (e *Environment) saveHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename := req.FormValue("filename")
	err := e.SaveCoinAddress(filename)
	if err != nil {
		fmt.Fprint(w, err)
	} else {
		fmt.Fprint(w, "Saved coin address to "+filename)
	}
}

func (e *Environment) loadHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename, friendname := req.FormValue("filename"), req.FormValue("friendname")
	err := e.LoadCoinAddress(filename, friendname)
	if err != nil {
		fmt.Fprint(w, err)
	} else {
		fmt.Fprint(w, "Loaded coin address to "+filename)
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
