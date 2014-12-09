package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"strings"
)

var tmpl = template.Must(template.ParseFiles("webpages/template.html"))

// webIndex loads a partial page according to the http request and composes it
// into a full page by adding the header and the footer, then serves the page.
// If there is an error, the error is reported (unsanitized). If the error is a
// 'partial file not found', a 404 (TODO) will be served.
func (e *Environment) webIndex(w http.ResponseWriter, req *http.Request) {
	// Parse the name of the partial file to load.
	var fileToLoad string
	fileRequested := string(req.URL.Path)
	if fileRequested == "/" {
		// Make a special case for the index.
		fileToLoad = "webpages/index.partial"
	} else {
		fileToLoad = "webpages" + strings.TrimSuffix(fileRequested, "html") + "partial"
	}

	// Load the partial file.
	indexBody, err := ioutil.ReadFile(fileToLoad)
	if err != nil {
		// TODO: serve a 404 if the file is not found.
		fmt.Fprintf(w, err.Error())
		fmt.Println(err)
		return
	}

	// Compose the partial into a full page and serve the page.
	tmpl.Execute(w, template.HTML(string(indexBody)))
}
