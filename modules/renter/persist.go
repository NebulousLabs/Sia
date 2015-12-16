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
	zip, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	defer zip.Close()
	enc := encoding.NewEncoder(zip)

	// encode easy fields
	err := enc.EncodeAll(
		f.name,
		f.size,
		f.masterKey,
		f.pieceSize,
		f.mode,
	)
	if err != nil {
		return err
	}
	// COMPATv0.4.3 - encode the bytesUploaded and chunksUploaded fields
	// TODO: the resulting .sia file may confuse old clients.
	err = enc.EncodeAll(f.pieceSize*f.numChunks()*uint64(f.erasureCode.NumPieces()), f.numChunks())
	if err != nil {
		return err
	}

	// encode erasureCode
	switch code := f.erasureCode.(type) {
	case *rsCode:
		err = enc.EncodeAll(
			"Reed-Solomon",
			uint64(code.dataPieces),
			uint64(code.numPieces-code.dataPieces),
		)
		if err != nil {
			return err
		}
	default:
		if build.DEBUG {
			panic("unknown erasure code")
		}
		return errors.New("unknown erasure code")
	}
	// encode contracts
	if err := enc.Encode(uint64(len(f.contracts))); err != nil {
		return err
	}
	for _, c := range f.contracts {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return nil
}

// load loads a file created by save.
func (f *file) load(r io.Reader) error {
	zip, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer zip.Close()
	dec := encoding.NewDecoder(zip)

	// COMPATv0.4.3 - decode bytesUploaded and chunksUploaded into dummy vars.
	var bytesUploaded, chunksUploaded uint64

	// decode easy fields
	err = dec.DecodeAll(
		&f.name,
		&f.size,
		&f.masterKey,
		&f.pieceSize,
		&f.mode,
		&bytesUploaded,
		&chunksUploaded,
	)
	if err != nil {
		return err
	}

	// decode erasure coder
	var codeType string
	if err := dec.Decode(&codeType); err != nil {
		return err
	}
	switch codeType {
	case "Reed-Solomon":
		var nData, nParity uint64
		err = dec.DecodeAll(
			&nData,
			&nParity,
		)
		if err != nil {
			return err
		}
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
	if err := dec.Decode(&nContracts); err != nil {
		return err
	}
	f.contracts = make(map[types.FileContractID]fileContract)
	var contract fileContract
	for i := uint64(0); i < nContracts; i++ {
		if err := dec.Decode(&contract); err != nil {
			return err
		}
		f.contracts[contract.ID] = contract
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

	// Write header with length of 1.
	err = encoding.NewEncoder(handle).EncodeAll(
		shareHeader,
		shareVersion,
		uint64(1),
	)
	if err != nil {
		return err
	}

	// Write file.
	err = f.save(handle)
	if err != nil {
		return err
	}

	// Commit the SafeFile.
	return handle.Commit()
}

// save stores the current renter data to disk.
func (r *Renter) save() error {
	data := struct {
		Tracking map[string]trackedFile
	}{r.tracking}
	return persist.SaveFile(saveMetadata, data, filepath.Join(r.persistDir, PersistFilename))
}

// load fetches the saved renter data from disk.
func (r *Renter) load() error {
	// Load all files found in renter directory.
	dir, err := os.Open(r.persistDir) // TODO: store in a subdir?
	if err != nil {
		return err
	}
	defer dir.Close()
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
		file.Close() // defer is probably a bad idea
		if err != nil {
			// maybe just skip?
			return err
		}
	}

	// Load contracts, repair set, and entropy.
	data := struct {
		Tracking  map[string]trackedFile
		Repairing map[string]string // COMPATv0.4.8
	}{}
	err = persist.LoadFile(saveMetadata, &data, filepath.Join(r.persistDir, PersistFilename))
	if err != nil {
		return err
	}
	if data.Tracking != nil {
		r.tracking = data.Tracking
	} else if data.Repairing != nil {
		// COMPATv0.4.8
		for nick, path := range data.Repairing {
			// these files will be renewed indefinitely
			r.tracking[nick] = trackedFile{RepairPath: path, Renew: true}
		}
	}

	return nil
}

// shareFiles writes the specified files to w.
func (r *Renter) shareFiles(nicknames []string, w io.Writer) error {
	// Write header.
	err := encoding.NewEncoder(w).EncodeAll(
		shareHeader,
		shareVersion,
		uint64(len(nicknames)),
	)
	if err != nil {
		return err
	}

	// Write each file.
	for _, name := range nicknames {
		file, exists := r.files[name]
		if !exists {
			return ErrUnknownNickname
		}
		err := file.save(w)
		if err != nil {
			return err
		}
	}

	return nil
}

// ShareFile saves the specified files to shareDest.
func (r *Renter) ShareFiles(nicknames []string, shareDest string) error {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// TODO: consider just appending the proper extension.
	if filepath.Ext(shareDest) != ShareExtension {
		return ErrNonShareSuffix
	}

	file, err := os.Create(shareDest)
	if err != nil {
		return err
	}
	defer file.Close()

	err = r.shareFiles(nicknames, file)
	if err != nil {
		os.Remove(shareDest)
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
	// read header
	var header [15]byte
	var version string
	var numFiles uint64
	err := encoding.NewDecoder(reader).DecodeAll(
		&header,
		&version,
		&numFiles,
	)
	if err != nil {
		return nil, err
	} else if header != shareHeader {
		return nil, ErrBadFile
	} else if version != shareVersion {
		return nil, ErrIncompatible
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
	// Save the files.
	for _, f := range files {
		r.saveFile(f)
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
