package siad

import (
	"fmt"
	"net/http"
	"os"

	"github.com/NebulousLabs/Andromeda/siacore"
)

func (e *Environment) stopHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: more graceful shutdown?
	e.Close()
	os.Exit(0)
}

func (e *Environment) catchupHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: check for existing CatchUp
	go e.CatchUp(e.RandomPeer())
}

func (e *Environment) mineHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: start/stop
	e.ToggleMining()
}

func (e *Environment) sendHandler(w http.ResponseWriter, req *http.Request) {
	var amount, fee siacore.Currency
	var dest siacore.CoinAddress
	// scan values
	// TODO: check error
	fmt.Sscan(req.FormValue("amount"), &amount)
	fmt.Sscan(req.FormValue("fee"), &fee)
	fmt.Sscan(req.FormValue("dest"), &dest)
	e.SpendCoins(amount, fee, dest)
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
	e.HostAnnounceSelf(freezeCoins, freezeBlocks+e.Height(), 10)
}

func (e *Environment) rentHandler(w http.ResponseWriter, req *http.Request) {
	filename := req.FormValue("filename")
	e.ClientProposeContract(filename)
}

func (e *Environment) downloadHandler(w http.ResponseWriter, req *http.Request) {
	filename := req.FormValue("filename")
	e.Download(filename)
}

func (e *Environment) saveHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename := req.FormValue("filename")
	e.SaveCoinAddress(filename)
}

func (e *Environment) loadHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: get type
	filename, friendname := req.FormValue("filename"), req.FormValue("friendname")
	e.LoadCoinAddress(filename, friendname)
}

func (e *Environment) statsHandler(w http.ResponseWriter, req *http.Request) {
	//w.Write("stats")
}

func (e *Environment) startServer() {
	// set up handlers
	http.HandleFunc("/catchup", e.catchupHandler)
	http.HandleFunc("/mine", e.mineHandler)
	http.HandleFunc("/send", e.sendHandler)
	http.HandleFunc("/host", e.hostHandler)
	http.HandleFunc("/rent", e.rentHandler)
	http.HandleFunc("/download", e.downloadHandler)
	http.HandleFunc("/save", e.saveHandler)
	http.HandleFunc("/load", e.loadHandler)
	http.HandleFunc("/stats", e.statsHandler)
	http.HandleFunc("/stop", e.stopHandler)
	// port should probably be an argument
	http.ListenAndServe(":9980", nil)
}
