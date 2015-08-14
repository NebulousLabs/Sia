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
	"github.com/NebulousLabs/Sia/types"
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
	defer zip.Close()
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
		dec.Decode(&contract)
		f.Contracts[contract.IP] = contract
	}
	return nil
}

// saveFile saves a file to the renter directory.
func (r *Renter) saveFile(f *file) error {
	handle, err := persist.NewSafeFile(filepath.Join(r.saveDir, f.Name+ShareExtension))
	if err != nil {
		return err
	}
	defer handle.Close()

	// Write length of 1.
	err = encoding.NewEncoder(handle).Encode(uint64(1))
	if err != nil {
		return err
	}

	// Write file.
	err = f.save(handle)
	if err != nil {
		return err
	}

	// Update file entry in renter.
	id := r.mu.Lock()
	r.files[f.Name] = f
	r.mu.Unlock(id)

	return r.save()
}

// save stores the current renter data to disk.
func (r *Renter) save() error {
	// Convert map to JSON-friendly type.
	contracts := make(map[string]types.FileContract)
	for id, fc := range r.contracts {
		b, _ := id.MarshalJSON()
		contracts[string(b)] = fc
	}
	return persist.SaveFile(saveMetadata, contracts, filepath.Join(r.saveDir, PersistFilename))
}

// load fetches the saved renter data from disk.
func (r *Renter) load() error {
	// Load contracts.
	contracts := make(map[string]types.FileContract)
	err := persist.LoadFile(saveMetadata, &contracts, filepath.Join(r.saveDir, PersistFilename))
	if err != nil {
		return err
	}
	var fcid types.FileContractID
	for id, fc := range contracts {
		fcid.UnmarshalJSON([]byte(id))
		r.contracts[fcid] = fc
	}

	// Load all files found in renter directory.
	f, err := os.Open(r.saveDir) // TODO: store in a subdir?
	if err != nil {
		return err
	}
	filenames, err := f.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, path := range filenames {
		// Skip non-sia files.
		if filepath.Ext(path) != ShareExtension {
			continue
		}
		file, err := os.Open(filepath.Join(r.saveDir, path))
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
	return nil
}

// shareFiles writes the specified files to w.
func (r *Renter) shareFiles(nicknames []string, w io.Writer) error {
	// Write number of files.
	err := encoding.NewEncoder(w).Encode(uint64(len(nicknames)))
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

	file, err := os.Open(sharedest)
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
	// Read number of files
	var numFiles uint64
	err := encoding.NewDecoder(reader).Decode(&numFiles)
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
		origName := files[i].Name
		for {
			_, exists := r.files[files[i].Name]
			if !exists {
				break
			}
			dupCount++
			files[i].Name = origName + "_" + strconv.Itoa(dupCount)
		}
	}

	// Add files to renter.
	names := make([]string, numFiles)
	for i, f := range files {
		r.files[f.Name] = f
		names[i] = f.Name
	}
	err = r.save()
	if err != nil {
		return nil, err
	}

	return names, nil
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
