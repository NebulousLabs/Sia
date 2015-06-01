package renter

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/persist"
)

const (
	PersistFilename = "renter.dat"
	ShareExtension  = ".sia"
)

var (
	ErrNoNicknames    = errors.New("at least one nickname must be supplied")
	ErrNonShareSuffix = errors.New("suffix of file must be " + ShareExtension)

	shareMetadata = persist.Metadata{
		Header:  "Sia Shared File",
		Version: "0.1",
	}

	saveMetadata = persist.Metadata{
		Header:  "Renter Persistence",
		Version: "0.2",
	}
)

// save stores the current renter data to disk.
func (r *Renter) save() error {
	var files []file
	for _, file := range r.files {
		files = append(files, *file)
	}
	return persist.SaveFile(saveMetadata, files, filepath.Join(r.saveDir, PersistFilename))
}

// load fetches the saved renter data from disk.
func (r *Renter) load() error {
	var files []file
	err := persist.LoadFile(saveMetadata, &files, filepath.Join(r.saveDir, PersistFilename))
	if err != nil {
		return err
	}
	for i := range files {
		files[i].renter = r
		r.files[files[i].Name] = &files[i]
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

	var files []file
	for _, nickname := range nicknames {
		file, exists := r.files[nickname]
		if !exists {
			return ErrUnknownNickname
		}
		active := 0
		for _, piece := range file.Pieces {
			if piece.Active {
				active++
			}
		}
		if active < 3 {
			return errors.New("Cannot share an inactive file")
		}
		files = append(files, *file)
	}

	// pipe data through json -> gzip -> w
	zip, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	err := persist.Save(shareMetadata, files, zip)
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
func (r *Renter) loadSharedFile(reader io.Reader) ([]string, error) {
	zip, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}

	var files []file
	err = persist.Load(shareMetadata, &files, zip)
	if err != nil {
		return nil, err
	}

	var fileList []string
	for i := range files {
		dupCount := 0
		origName := files[i].Name
		for {
			_, exists := r.files[files[i].Name]
			if !exists {
				break
			}
			dupCount++
			files[i].Name = origName + "_" + strconv.Itoa(dupCount)
		}
		files[i].renter = r
		r.files[files[i].Name] = &files[i]
		fileList = append(fileList, files[i].Name)
	}
	r.save()

	return fileList, nil
}

// LoadSharedFile loads a shared file into the renter.
func (r *Renter) LoadSharedFile(filename string) ([]string, error) {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return r.loadSharedFile(file)
}

// loadSharedFile takes an encoded set of files and adds them to the renter,
// taking them form an ascii string.
func (r *Renter) LoadSharedFilesAscii(asciiSia string) ([]string, error) {
	dec := base64.NewDecoder(base64.URLEncoding, bytes.NewBufferString(asciiSia))
	return r.loadSharedFile(dec)
}
