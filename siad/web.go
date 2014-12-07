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
	indexBody, err := ioutil.ReadFile("webpages/index.partial")
	if err != nil {
		fmt.Println(err)
	}
	page, err := webComposePage(indexBody)
	if err != nil {
		fmt.Fprint(w, err)
	}
	fmt.Fprintf(w, "%s", page)
}
