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

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	PersistFilename = "renter.json"
	ShareExtension  = ".sia"
)

var (
	ErrNoNicknames    = errors.New("at least one nickname must be supplied")
	ErrNonShareSuffix = errors.New("suffix of file must be " + ShareExtension)

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
	enc := encoding.NewEncoder(zip)

	// encode easy fields
	enc.Encode(shareVersion)
	enc.Encode(f.Name)
	enc.Encode(f.Size)
	enc.Encode(f.MasterKey)
	enc.Encode(f.pieceSize)
	enc.Encode(f.bytesUploaded)
	enc.Encode(f.chunksUploaded)

	// encode ecc
	switch code := f.ecc.(type) {
	case *rsCode:
		enc.Encode("Reed-Solomon")
		enc.Encode(uint64(code.dataPieces))
		enc.Encode(uint64(code.numPieces - code.dataPieces))
	default:
		panic("unknown ECC")
	}
	// encode contracts
	enc.Encode(uint64(len(f.Contracts)))
	for _, c := range f.Contracts {
		enc.Encode(c.ID)
		enc.Encode(c.Pieces)
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
	dec := encoding.NewDecoder(zip)

	// decode version
	var version string
	dec.Decode(&version)
	if version != shareVersion {
		return errors.New("incompatible version: " + version)
	}

	// decode easy fields
	dec.Decode(&f.Name)
	dec.Decode(&f.Size)
	dec.Decode(&f.MasterKey)
	dec.Decode(&f.pieceSize)
	dec.Decode(&f.bytesUploaded)
	dec.Decode(&f.chunksUploaded)

	// decode ecc
	var eccID string
	dec.Decode(&eccID)
	switch eccID {
	case "Reed-Solomon":
		var nData, nParity uint64
		dec.Decode(&nData)
		dec.Decode(&nParity)
		ecc, err := NewRSCode(int(nData), int(nParity))
		if err != nil {
			return err
		}
		f.ecc = ecc
	default:
		return errors.New("unrecognized ECC type: " + eccID)
	}

	// decode contracts
	var nContracts uint64
	dec.Decode(&nContracts)
	f.Contracts = make(map[modules.NetAddress]fileContract)
	var contract fileContract
	for i := uint64(0); i < nContracts; i++ {
		dec.Decode(&contract.ID)
		dec.Decode(&contract.Pieces)
		f.Contracts[contract.IP] = contract
	}
	return nil
}

// save stores the current renter data to disk.
func (r *Renter) save() error {
	return persist.SaveFile(saveMetadata, r.contracts, filepath.Join(r.saveDir, PersistFilename))
}

// load fetches the saved renter data from disk.
func (r *Renter) load() error {
	err := persist.LoadFile(saveMetadata, &r.contracts, filepath.Join(r.saveDir, PersistFilename))
	if err != nil {
		return err
	}
	// load all files found in renter directory
	f, err := os.Open(r.saveDir) // TODO: store in a subdir?
	if err != nil {
		return err
	}
	filenames, err := f.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, path := range filenames {
		file, err := os.Open(path)
		if err != nil {
			// maybe just skip?
			return err
		}
		_, err = r.loadSharedFile(file)
		if err != nil {
			// maybe just skip?
			return err
		}
	}
	return nil
}

// ShareFileAscii returns the named file in ASCII format.
func (r *Renter) ShareFileAscii(nickname string) (string, error) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	file, exists := r.files[nickname]
	if !exists {
		return "", ErrUnknownNickname
	}

	// pipe to a base64 encoder
	buf := new(bytes.Buffer)
	err := file.save(base64.NewEncoder(base64.URLEncoding, buf))
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// loadSharedFile reads a shared file from reader and registers it in the
// renter. It returns the nickname of the loaded file.
func (r *Renter) loadSharedFile(reader io.Reader) (string, error) {
	f := new(file)
	err := f.load(reader)
	if err != nil {
		return "", err
	}

	// Make sure the file's name does not conflict with existing files.
	dupCount := 0
	origName := f.Name
	for {
		_, exists := r.files[f.Name]
		if !exists {
			break
		}
		dupCount++
		f.Name = origName + "_" + strconv.Itoa(dupCount)
	}

	// Add file to renter.
	r.files[f.Name] = f
	err = r.save()
	if err != nil {
		return err
	}

	return f.Name, nil
}

// LoadSharedFile loads a shared file into the renter. It returns the nickname
// of the loaded file.
func (r *Renter) LoadSharedFile(filename string) (string, error) {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	return r.loadSharedFile(file)
}

// LoadSharedFileAscii loads an ASCII-encoded file into the renter. It returns
// the nickname of the loaded file.
func (r *Renter) LoadSharedFileAscii(asciiSia string) (string, error) {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	dec := base64.NewDecoder(base64.URLEncoding, bytes.NewBufferString(asciiSia))
	return r.loadSharedFile(dec)
}
