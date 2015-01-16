package main

import (
	"net/http"
)

// Everything you need can be found in sia/renter.go and
// sia/components/renter.go

func (d *daemon) fileUploadHandler(w http.ResponseWriter, req *http.Request) {
	// Need to pull a byte slice from the upload stuffs.
	d.core.RentSmallFile(components.RentSmallFileParameters{
		FullFile: []byte
		Nickname string
		TotalPieces: int
	})
}

func (d *daemon) fileDownloadHandler(w http.ResponseWriter, req *http.Request) {
	var nickname string

	// scan something into the string

	err = d.core.RenterDownload(nickname, d.downloadDir)
	// write the error
}

func (d *daemon) fileStatusHandler(w http.ResponseWriter, req *http.Request) {
	info, err := d.core.RentInfo()
	if err != nil {
		// write the error
	}

	// Jsonify the info
	// Write it.
}
