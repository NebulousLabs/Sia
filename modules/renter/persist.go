package renter

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
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

	jsonBytes, err := json.Marshal(rp)
	if err != nil {
		return err
	}

	// Write everything to disk.
	err = ioutil.WriteFile(filepath.Join(r.saveDir, PersistFilename), jsonBytes, 0660)
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

// shareFiles encodes a set of file nicknames into a byte slice that can be
// shared with other daemons, giving them access to those files.
func (r *Renter) shareFiles(nicknames []string) (zipBytes []byte, err error) {
	if len(nicknames) == 0 {
		return nil, ErrNoNicknames
	}

	rsf := RenterSharedFile{
		Header:  ShareHeader,
		Version: ShareVersion,
		Files:   make([]file, 0, len(nicknames)),
	}
	for _, nickname := range nicknames {
		file, exists := r.files[nickname]
		if !exists {
			return nil, ErrUnknownNickname
		}
		rsf.Files = append(rsf.Files, *file)
	}

	shareBytes, err := json.Marshal(rsf)
	if err != nil {
		return nil, err
	}

	// Gzip the result.
	var zipBuffer bytes.Buffer
	zip, err := gzip.NewWriterLevel(&zipBuffer, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	_, err = zip.Write(shareBytes)
	if err != nil {
		return nil, err
	}
	zip.Close()

	return zipBuffer.Bytes(), nil
}

// ShareFiles saves a '.sia' file that can be shared with others, enabling them
// to download the file you are sharing. It creates a Sia equivalent of a
// '.torrent'.
func (r *Renter) ShareFiles(nicknames []string, sharedest string) error {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// Suffix enforcement is not really necessary, but I want explicit
	// enforcement of the suffix until people are used to seeing '.sia' files.
	if filepath.Ext(sharedest) != ShareExtension {
		return ErrNonShareSuffix
	}

	shareBytes, err := r.shareFiles(nicknames)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(sharedest, shareBytes, 0660)
	if err != nil {
		return err
	}
	return nil
}

// ShareFilesAscii returns an ascii string that can be shared with other
// daemons, granting them access to the files.
func (r *Renter) ShareFilesAscii(nicknames []string) (asciiSia string, err error) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	shareBytes, err := r.shareFiles(nicknames)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(shareBytes), nil
}

// loadSharedFile takes an encoded set of files and adds them to the renter.
func (r *Renter) loadSharedFile(zipBytes []byte) error {
	// Un-gzip the contents.
	var unzipBuffer bytes.Buffer
	zipBuffer := bytes.NewBuffer(zipBytes)
	zip, err := gzip.NewReader(zipBuffer)
	if err != nil {
		return err
	}
	io.Copy(&unzipBuffer, zip)
	shareBytes := unzipBuffer.Bytes()

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

// LoadSharedFile loads a shared file into the renter.
func (r *Renter) LoadSharedFile(filename string) error {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	shareBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	return r.loadSharedFile(shareBytes)
}

// loadSharedFile takes an encoded set of files and adds them to the renter,
// taking them form an ascii string.
func (r *Renter) LoadSharedFilesAscii(asciiSia string) error {
	shareBytes, err := base64.URLEncoding.DecodeString(asciiSia)
	if err != nil {
		return err
	}
	return r.loadSharedFile(shareBytes)
}
