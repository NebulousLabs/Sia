package main

import (
	"net/http"
)

func main() {
	// create environment
	e, err := CreateEnvironment(9989, true)
	if err != nil {
		println(err.Error())
		return
	}

	// set up handlers
	http.HandleFunc("/catchup", e.catchupHandler)
	http.HandleFunc("/mine", e.mineHandler)
	http.HandleFunc("/send", e.sendHandler)
	http.HandleFunc("/host", e.hostHandler)
	http.HandleFunc("/rent", e.rentHandler)
	http.HandleFunc("/download", e.downloadHandler)
	http.HandleFunc("/save", e.saveHandler)
	http.HandleFunc("/load", e.loadHandler)
	http.HandleFunc("/status", e.statusHandler)
	http.HandleFunc("/stop", e.stopHandler)
	// port should probably be an argument
	// TODO: timeouts?
	http.ListenAndServe(":9980", nil)
}
