package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
)

// webIndex loads a partial page according to the http request and composes it
// into a full page by adding the header and the footer, then serves the page.
func (d *daemon) webIndex(w http.ResponseWriter, req *http.Request) {
	// Parse the name of the partial file to load.
	var fileToLoad string
	if req.URL.Path == "/" {
		// Make a special case for the index.
		fileToLoad = path.Join(d.styleDir, "index.html")
	} else {
		fileToLoad = path.Join(d.styleDir, "index.html#") + req.URL.Path
	}

	// Load the partial file.
	indexBody, err := ioutil.ReadFile(fileToLoad)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	// Compose the partial into a full page and serve the page.
	fmt.Fprintf(w, "%s", indexBody)
}
