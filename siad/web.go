package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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

// webIndex loads a partial page according to the http request and composes it
// into a full page by adding the header and the footer, then serves the page.
// If there is an error, the error is reported (unsanitized). If the error is a
// 'partial file not found', a 404 (TODO) will be served.
func (e *Environment) webIndex(w http.ResponseWriter, req *http.Request) {
	// Load the appropriate partial file into memory.
	fileRequested := string(req.URL.Path)
	fileToLoad := "webpages" + strings.TrimSuffix(fileRequested, "html") + "partial"
	indexBody, err := ioutil.ReadFile(fileToLoad)
	if err != nil {
		// TODO: serve a 404 if the file is not found.
		fmt.Fprintf(w, err.Error())
		fmt.Println(err)
		return
	}

	// Compose the partial into a full page and serve the page.
	page, err := webComposePage(indexBody)
	if err != nil {
		fmt.Fprint(w, err)
		fmt.Println(err)
		return
	}
	fmt.Fprintf(w, "%s", page)
}
