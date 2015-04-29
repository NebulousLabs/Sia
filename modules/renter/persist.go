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

	ShareExtension = ".sia"
	ShareHeader    = "Sia Shared File"
	ShareVersion   = "0.1"
)

var (
	ErrUnrecognizedHeader  = errors.New("renter persistence file has unrecognized header")
	ErrUnrecognizedVersion = errors.New("renter persistence file has unrecognized version")

	ErrNoNicknames    = errors.New("at least one nickname must be supplied")
	ErrNonShareSuffix = errors.New("suffix of file must be " + ShareExtension)
)

// RenterPersistence is the struct that gets written to and read from disk as
// the renter is saved and loaded.
type RenterPersistence struct {
	Header  string
	Version string
	Files   []file
}

// RenterSharedFile is the struct that gets written to and read from disk when
// sharing files.
type RenterSharedFile struct {
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
		rp.Files = append(rp.Files, *file)
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
	persistBytes, err := ioutil.ReadFile(filepath.Join(r.saveDir, PersistFilename))
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
	for i := range rp.Files {
		rp.Files[i].renter = r
		r.files[rp.Files[i].Name] = &rp.Files[i]
	}
	return nil
}

// ShareFiles saves a '.sia' file that can be shared with others, enabling them
// to download the file you are sharing. It creates a Sia equivalent of a
// '.torrent'.
func (r *Renter) ShareFiles(nicknames []string, sharedest string) error {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	if len(nicknames) == 0 {
		return ErrNoNicknames
	}
	// Suffix enforcement is not really necessary, but I want explicit
	// enforcement of the suffix until people are used to seeing '.sia' files.
	if filepath.Ext(sharedest) != ShareExtension {
		return ErrNonShareSuffix
	}

	rsf := RenterSharedFile{
		Header:  ShareHeader,
		Version: ShareVersion,
		Files:   make([]file, 0, len(nicknames)),
	}
	for _, nickname := range nicknames {
		file, exists := r.files[nickname]
		if !exists {
			return ErrUnknownNickname
		}
		rsf.Files = append(rsf.Files, *file)
	}

	shareBytes, err := json.Marshal(rsf)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(sharedest, shareBytes, 0660)
	if err != nil {
		return err
	}
	return nil
}

// LoadSharedFile loads a shared file into the renter.
func (r *Renter) LoadSharedFile(filename string) error {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	shareBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	var rsf RenterSharedFile
	err = json.Unmarshal(shareBytes, &rsf)
	if err != nil {
		return err
	}

	if rsf.Header != ShareHeader {
		return ErrUnrecognizedHeader
	}
	if rsf.Version != ShareVersion {
		return ErrUnrecognizedVersion
	}
	for i := range rsf.Files {
		rsf.Files[i].renter = r
		r.files[rsf.Files[i].Name] = &rsf.Files[i]
	}
	r.save()
	return nil
}
