package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

// composePage adds the header and the footer to the given byte slice, and
// returns the result.
func webComposePage(body []byte) (page []byte, err error) {
	// Read the header and footer into memory.
	header, err := ioutil.ReadFile("webpages/header.partial")
	if err != nil {
		return
	}
	footer, err := ioutil.ReadFile("webpages/footer.partial")
	if err != nil {
		return
	}

	page = append(append(header, body...), footer...)
	return
}

func (e *Environment) webIndex(w http.ResponseWriter, req *http.Request) {
	// Figure out which partial file to load.
	var fileToLoad string
	fileRequested := string(req.URL.Path)
	if fileRequested == "/" || fileRequested == "/index.html" {
		fileToLoad = "webpages/index.partial"
	} else if fileRequested == "/mine.html" {
		fileToLoad = "webpages/mine.partial"
	} else if fileRequested == "/wallet.html" {
		fileToLoad = "webpages/wallet.partial"
	} else {
		fmt.Fprint(w, "unrecognized page request")
		return
	}

	// Load the partial file, compose the webpage, and serve the webpage.
	indexBody, err := ioutil.ReadFile(fileToLoad)
	if err != nil {
		fmt.Fprintf(w, err.Error())
		fmt.Println(err)
		return
	}
	page, err := webComposePage(indexBody)
	if err != nil {
		fmt.Fprint(w, err)
		fmt.Println(err)
		return
	}
	fmt.Fprintf(w, "%s", page)
}
