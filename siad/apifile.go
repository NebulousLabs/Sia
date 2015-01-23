package main

import (
	// "io/ioutil"
	"net/http"
	// "path/filepath"
	// "strconv"

	// "github.com/NebulousLabs/Sia/sia/components"
)

func (d *daemon) fileUploadHandler(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "Unimplemented", 500)
	/*
		pieces, err := strconv.Atoi(req.FormValue("pieces"))
		if err != nil {
			http.Error(w, "Malformed pieces", 400)
			return
		}

		// this is slightly dangerous, but we assume the user won't try to attack siad
		file, _, err := req.FormFile("file")
		if err != nil {
			http.Error(w, "Malformed/missing file: "+err.Error(), 400)
			return
		}
		defer file.Close()

		data, err := ioutil.ReadAll(file)
		if err != nil {
			http.Error(w, "Could not read file data: "+err.Error(), 400)
			return
		}

		// TODO: is "" a valid nickname? The renter should probably prevent this.
		err = d.core.RentSmallFile(components.RentSmallFileParameters{
			FullFile:    data,
			Nickname:    req.FormValue("nickname"),
			TotalPieces: pieces,
		})
		if err != nil {
			http.Error(w, "Upload failed: "+err.Error(), 500)
			return
		}

		writeSuccess(w)
	*/
}

func (d *daemon) fileUploadPathHandler(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "Unimplemented", 500)
	/*
		pieces, err := strconv.Atoi(req.FormValue("pieces"))
		if err != nil {
			http.Error(w, "Malformed pieces", 400)
			return
		}

		// TODO: is "" a valid nickname? The renter should probably prevent this.
		err = d.core.RentFile(components.RentFileParameters{
			Filepath:    req.FormValue("filename"),
			Nickname:    req.FormValue("nickname"),
			TotalPieces: pieces,
		})
		if err != nil {
			http.Error(w, "Upload failed: "+err.Error(), 500)
			return
		}

		writeSuccess(w)
	*/
}

func (d *daemon) fileDownloadHandler(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "Unimplemented", 500)
	/*
			err := d.core.RenterDownload(req.FormValue("nickname"), filepath.Join(d.downloadDir, req.FormValue("filename")))
			if err != nil {
				// TODO: if this err is a user error (e.g. bad nickname), return 400 instead
				http.Error(w, "Download failed: "+err.Error(), 500)
				return
			}

			writeSuccess(w)
		}
	*/
}

func (d *daemon) fileStatusHandler(w http.ResponseWriter, req *http.Request) {
	http.Error(w, "Unimplemented", 500)
	/*
		info, err := d.core.RentInfo()
		if err != nil {
			http.Error(w, "Couldn't get renter info: "+err.Error(), 500)
			return
		}

		writeJSON(w, info)
	*/
}
