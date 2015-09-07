package renter

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	PersistFilename = "renter.json"
	ShareExtension  = ".sia"
)

var (
	ErrNoNicknames    = errors.New("at least one nickname must be supplied")
	ErrNonShareSuffix = errors.New("suffix of file must be " + ShareExtension)
	ErrBadFile        = errors.New("not a .sia file")
	ErrIncompatible   = errors.New("file is not compatible with current version")

	shareHeader  = [15]byte{'S', 'i', 'a', ' ', 'S', 'h', 'a', 'r', 'e', 'd', ' ', 'F', 'i', 'l', 'e'}
	shareVersion = "0.4"

	saveMetadata = persist.Metadata{
		Header:  "Renter Persistence",
		Version: "0.4",
	}
)

// save saves a file to w in shareable form. Files are stored in binary format
// and gzipped to reduce size.
func (f *file) save(w io.Writer) error {
	// TODO: error checking
	zip, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	defer zip.Close()
	enc := encoding.NewEncoder(zip)

	// encode easy fields
	enc.Encode(f.name)
	enc.Encode(f.size)
	enc.Encode(f.masterKey)
	enc.Encode(f.pieceSize)
	enc.Encode(f.mode)
	enc.Encode(f.bytesUploaded)
	enc.Encode(f.chunksUploaded)

	// encode erasureCode
	switch code := f.erasureCode.(type) {
	case *rsCode:
		enc.Encode("Reed-Solomon")
		enc.Encode(uint64(code.dataPieces))
		enc.Encode(uint64(code.numPieces - code.dataPieces))
	default:
		if build.DEBUG {
			panic("unknown erasure code")
		}
		return errors.New("unknown erasure code")
	}
	// encode contracts
	enc.Encode(uint64(len(f.contracts)))
	for _, c := range f.contracts {
		enc.Encode(c)
	}
	return nil
}

// load loads a file created by save.
func (f *file) load(r io.Reader) error {
	// TODO: error checking
	zip, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer zip.Close()
	dec := encoding.NewDecoder(zip)

	// decode easy fields
	dec.Decode(&f.name)
	dec.Decode(&f.size)
	dec.Decode(&f.masterKey)
	dec.Decode(&f.pieceSize)
	dec.Decode(&f.mode)
	dec.Decode(&f.bytesUploaded)
	dec.Decode(&f.chunksUploaded)

	// decode erasure coder
	var codeType string
	dec.Decode(&codeType)
	switch codeType {
	case "Reed-Solomon":
		var nData, nParity uint64
		dec.Decode(&nData)
		dec.Decode(&nParity)
		rsc, err := NewRSCode(int(nData), int(nParity))
		if err != nil {
			return err
		}
		f.erasureCode = rsc
	default:
		return errors.New("unrecognized erasure code type: " + codeType)
	}

	// decode contracts
	var nContracts uint64
	dec.Decode(&nContracts)
	f.contracts = make(map[modules.NetAddress]fileContract)
	var contract fileContract
	for i := uint64(0); i < nContracts; i++ {
		dec.Decode(&contract)
		f.contracts[contract.IP] = contract
	}
	return nil
}

// saveFile saves a file to the renter directory.
func (r *Renter) saveFile(f *file) error {
	handle, err := persist.NewSafeFile(filepath.Join(r.persistDir, f.name+ShareExtension))
	if err != nil {
		return err
	}
	defer handle.Close()

	enc := encoding.NewEncoder(handle)

	// Write header.
	enc.Encode(shareHeader)
	enc.Encode(shareVersion)

	// Write length of 1.
	err = enc.Encode(uint64(1))
	if err != nil {
		return err
	}

	// Write file.
	err = f.save(handle)
	if err != nil {
		return err
	}

	return nil
}

// save stores the current renter data to disk.
func (r *Renter) save() error {
	data := struct {
		Contracts map[string]types.FileContract
		Entropy   [32]byte
	}{make(map[string]types.FileContract), r.entropy}
	// Convert renter's contract map to a JSON-friendly type.
	for id, fc := range r.contracts {
		b, _ := id.MarshalJSON()
		data.Contracts[string(b)] = fc
	}
	return persist.SaveFile(saveMetadata, data, filepath.Join(r.persistDir, PersistFilename))
}

// load fetches the saved renter data from disk.
func (r *Renter) load() error {
	// Load all files found in renter directory.
	dir, err := os.Open(r.persistDir) // TODO: store in a subdir?
	if err != nil {
		return err
	}
	filenames, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, path := range filenames {
		// Skip non-sia files.
		if filepath.Ext(path) != ShareExtension {
			continue
		}
		file, err := os.Open(filepath.Join(r.persistDir, path))
		if err != nil {
			// maybe just skip?
			return err
		}
		_, err = r.loadSharedFiles(file)
		if err != nil {
			// maybe just skip?
			return err
		}
	}

	// Load contracts and entropy.
	data := struct {
		Contracts map[string]types.FileContract
		Entropy   [32]byte
	}{}
	err = persist.LoadFile(saveMetadata, &data, filepath.Join(r.persistDir, PersistFilename))
	if err != nil {
		return err
	}
	r.entropy = data.Entropy
	var fcid types.FileContractID
	for id, fc := range data.Contracts {
		fcid.UnmarshalJSON([]byte(id))
		r.contracts[fcid] = fc
	}

	return nil
}

// shareFiles writes the specified files to w.
func (r *Renter) shareFiles(nicknames []string, w io.Writer) error {
	enc := encoding.NewEncoder(w)

	// Write header.
	enc.Encode(shareHeader)
	enc.Encode(shareVersion)

	// Write number of files.
	err := enc.Encode(uint64(len(nicknames)))
	if err != nil {
		return err
	}

	// Write each file.
	for _, name := range nicknames {
		file, exists := r.files[name]
		if !exists {
			return ErrUnknownNickname
		}
		err = file.save(w)
		if err != nil {
			return err
		}
	}

	return nil
}

// ShareFile saves the specified files to sharedest.
func (r *Renter) ShareFiles(nicknames []string, sharedest string) error {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// TODO: consider just appending the proper extension.
	if filepath.Ext(sharedest) != ShareExtension {
		return ErrNonShareSuffix
	}

	file, err := os.Create(sharedest)
	if err != nil {
		return err
	}
	defer file.Close()

	err = r.shareFiles(nicknames, file)
	if err != nil {
		os.Remove(sharedest)
		return err
	}

	return nil
}

// ShareFilesAscii returns the specified files in ASCII format.
func (r *Renter) ShareFilesAscii(nicknames []string) (string, error) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	buf := new(bytes.Buffer)
	err := r.shareFiles(nicknames, base64.NewEncoder(base64.URLEncoding, buf))
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// loadSharedFiles reads .sia data from reader and registers the contained
// files in the renter. It returns the nicknames of the loaded files.
func (r *Renter) loadSharedFiles(reader io.Reader) ([]string, error) {
	dec := encoding.NewDecoder(reader)

	// read header
	var header [15]byte
	dec.Decode(&header)
	if header != shareHeader {
		return nil, ErrBadFile
	}

	// decode version
	var version string
	dec.Decode(&version)
	if version != shareVersion {
		return nil, ErrIncompatible
	}

	// Read number of files
	var numFiles uint64
	err := dec.Decode(&numFiles)
	if err != nil {
		return nil, err
	}

	// Read each file.
	files := make([]*file, numFiles)
	for i := range files {
		files[i] = new(file)
		err := files[i].load(reader)
		if err != nil {
			return nil, err
		}

		// Make sure the file's name does not conflict with existing files.
		dupCount := 0
		origName := files[i].name
		for {
			_, exists := r.files[files[i].name]
			if !exists {
				break
			}
			dupCount++
			files[i].name = origName + "_" + strconv.Itoa(dupCount)
		}
	}

	// Add files to renter.
	names := make([]string, numFiles)
	for i, f := range files {
		r.files[f.name] = f
		names[i] = f.name
	}
	// Save the files, and their renter metadata.
	for _, f := range files {
		r.saveFile(f)
	}
	err = r.save()
	if err != nil {
		return nil, err
	}

	return names, nil
}

// initPersist handles all of the persistence initialization, such as creating
// the persistance directory and starting the logger.
func (r *Renter) initPersist() error {
	// Create the perist directory if it does not yet exist.
	err := os.MkdirAll(r.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	logFile, err := os.OpenFile(filepath.Join(r.persistDir, "renter.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	r.log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	r.log.Println("STARTUP: Renter has started logging")

	// Load the prior persistance structures.
	err = r.load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadSharedFiles loads a .sia file into the renter. It returns the nicknames
// of the loaded files.
func (r *Renter) LoadSharedFiles(filename string) ([]string, error) {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return r.loadSharedFiles(file)
}

// LoadSharedFilesAscii loads an ASCII-encoded .sia file into the renter. It
// returns the nicknames of the loaded files.
func (r *Renter) LoadSharedFilesAscii(asciiSia string) ([]string, error) {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	dec := base64.NewDecoder(base64.URLEncoding, bytes.NewBufferString(asciiSia))
	return r.loadSharedFiles(dec)
}
