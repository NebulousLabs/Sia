package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	duration = 2000
	delay    = 20
)

func (d *daemon) fileUploadHandler(w http.ResponseWriter, req *http.Request) {
	pieces, err := strconv.Atoi(req.FormValue("pieces"))
	if err != nil {
		http.Error(w, "Malformed pieces", 400)
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		http.Error(w, "Malformed/missing file: "+err.Error(), 400)
		return
	}
	defer file.Close()

	// calculate filesize (via Seek; 2 means "relative to the end")
	n, _ := file.Seek(0, 2)
	filesize := uint64(n)
	file.Seek(0, 0) // reset

	// calculate Merkle root
	merkle, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(filesize))
	if err != nil {
		http.Error(w, "Couldn't calculate Merkle root: "+err.Error(), 500)
		return
	}

	err = d.renter.Upload(modules.UploadParams{
		Data:       file,
		FileSize:   filesize,
		MerkleRoot: merkle,

		// TODO: the user should probably supply these
		Duration: duration,
		Delay:    delay,

		Nickname:    req.FormValue("nickname"),
		TotalPieces: pieces,
	})
	if err != nil {
		http.Error(w, "Upload failed: "+err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) fileUploadPathHandler(w http.ResponseWriter, req *http.Request) {
	pieces, err := strconv.Atoi(req.FormValue("pieces"))
	if err != nil {
		http.Error(w, "Malformed pieces", 400)
		return
	}

	// open the file
	file, err := os.Open(req.FormValue("filename"))
	if err != nil {
		http.Error(w, "Couldn't open file: "+err.Error(), 400)
		return
	}

	// calculate filesize
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "Couldn't stat file: "+err.Error(), 400)
		return
	}
	filesize := uint64(info.Size())

	// calculate Merkle root
	merkle, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(uint64(info.Size())))
	if err != nil {
		http.Error(w, "Couldn't calculate Merkle root: "+err.Error(), 500)
		return
	}

	err = d.renter.Upload(modules.UploadParams{
		Data:       file,
		FileSize:   filesize,
		MerkleRoot: merkle,

		// TODO: the user should probably supply these
		Duration: duration,
		Delay:    delay,

		Nickname:    req.FormValue("nickname"),
		TotalPieces: pieces,
	})
	if err != nil {
		http.Error(w, "Upload failed: "+err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) fileDownloadHandler(w http.ResponseWriter, req *http.Request) {
	path := filepath.Join(d.downloadDir, req.FormValue("filename"))
	err := d.renter.Download(req.FormValue("nickname"), path)
	if err != nil {
		// TODO: if this err is a user error (e.g. bad nickname), return 400 instead
		http.Error(w, "Download failed: "+err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) fileStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.renter.Info())
}
