package renter

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
)

const (
	PersistFilename = "renter.dat"
	PersistHeader   = "Renter Persistence"
	PersistVersion  = "0.2"
)

var (
	ErrUnrecognizedHeader  = errors.New("renter persistence file has unrecognized header")
	ErrUnrecognizedVersion = errors.New("renter persistence file has unrecognized version")
)

// RenterPersistence is the struct that gets written to and read from disk as
// the renter is saved and loaded.
type RenterPersistence struct {
	Header  string
	Version string
	Files   []file
}

// save stores the current renter data to disk.
func (r *Renter) save() error {
	rp := RenterPersistence{
		Header:  PersistHeader,
		Version: PersistVersion,
		Files:   make([]file, 0, len(r.files)),
	}
	for _, file := range r.files {
		rp.Files = append(rp.Files, file)
	}

	persistBytes, err := json.Marshal(rp)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(r.saveDir, PersistFilename), persistBytes, 0660)
	if err != nil {
		return err
	}
	return nil
}

// load fetches the saved renter data from disk.
func (r *Renter) load() error {
	persistBytes, err := ioutil.ReadFile(filepath.Join(r.saveDir, "files.dat"))
	if err != nil {
		return err
	}
	var rp RenterPersistence
	err = json.Unmarshal(persistBytes, &rp)
	if err != nil {
		return err
	}

	if rp.Header != PersistHeader {
		return ErrUnrecognizedHeader
	}
	if rp.Version != PersistVersion {
		return ErrUnrecognizedVersion
	}
	for _, file := range rp.Files {
		r.files[file.Name] = file
	}
	return nil
}
