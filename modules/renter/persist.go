package renter

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
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

	file, err := os.Create(filepath.Join(r.saveDir, PersistFilename))
	if err != nil {
		return err
	}

	err = json.NewEncoder(file).Encode(rp)
	if err != nil {
		return err
	}

	return nil
}

// load fetches the saved renter data from disk.
func (r *Renter) load() error {
	file, err := os.Open(filepath.Join(r.saveDir, PersistFilename))
	if err != nil {
		return err
	}

	var rp RenterPersistence
	err = json.NewDecoder(file).Decode(&rp)
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

// shareFiles writes the metadata of each file specified by nicknames to w.
// This output can be shared with other daemons, giving them access to those
// files.
func (r *Renter) shareFiles(nicknames []string, w io.Writer) error {
	if len(nicknames) == 0 {
		return ErrNoNicknames
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

	// pipe data through json -> gzip -> w
	zip, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	err := json.NewEncoder(zip).Encode(rsf)
	if err != nil {
		return err
	}
	zip.Close()

	return nil
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

	file, err := os.Create(sharedest)
	if err != nil {
		return err
	}

	return r.shareFiles(nicknames, file)
}

// ShareFilesAscii returns an ascii string that can be shared with other
// daemons, granting them access to the files.
func (r *Renter) ShareFilesAscii(nicknames []string) (string, error) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// pipe to a base64 encoder
	buf := new(bytes.Buffer)
	err := r.shareFiles(nicknames, base64.NewEncoder(base64.URLEncoding, buf))
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// loadSharedFile reads and decodes file metadata from reader and adds it to
// the renter.
func (r *Renter) loadSharedFile(reader io.Reader) error {
	zip, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}

	var rsf RenterSharedFile
	err = json.NewDecoder(zip).Decode(&rsf)
	if err != nil {
		return err
	}

	if rsf.Header != ShareHeader {
		return ErrUnrecognizedHeader
	} else if rsf.Version != ShareVersion {
		return ErrUnrecognizedVersion
	}
	for i := range rsf.Files {
		for {
			_, exists := r.files[rsf.Files[i].Name]
			if !exists {
				break
			}
			if len(rsf.Files[i].Name) > 50 {
				break
			}
			rsf.Files[i].Name += "_"
		}
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

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	return r.loadSharedFile(file)
}

// loadSharedFile takes an encoded set of files and adds them to the renter,
// taking them form an ascii string.
func (r *Renter) LoadSharedFilesAscii(asciiSia string) error {
	dec := base64.NewDecoder(base64.URLEncoding, bytes.NewBufferString(asciiSia))
	return r.loadSharedFile(dec)
}
