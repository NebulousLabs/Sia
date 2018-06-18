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
	"github.com/NebulousLabs/Sia/modules/renter/siafile"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	logFile = modules.RenterDir + ".log"
	// PersistFilename is the filename to be used when persisting renter information to a JSON file
	PersistFilename = "renter.json"
	// ShareExtension is the extension to be used
	ShareExtension = ".sia"
)

var (
	//ErrBadFile is an error when a file does not qualify as .sia file
	ErrBadFile = errors.New("not a .sia file")
	// ErrIncompatible is an error when file is not compatible with current version
	ErrIncompatible = errors.New("file is not compatible with current version")
	// ErrNoNicknames is an error when no nickname is given
	ErrNoNicknames = errors.New("at least one nickname must be supplied")
	// ErrNonShareSuffix is an error when the suffix of a file does not match the defined share extension
	ErrNonShareSuffix = errors.New("suffix of file must be " + ShareExtension)

	settingsMetadata = persist.Metadata{
		Header:  "Renter Persistence",
		Version: persistVersion,
	}

	shareHeader  = [15]byte{'S', 'i', 'a', ' ', 'S', 'h', 'a', 'r', 'e', 'd', ' ', 'F', 'i', 'l', 'e'}
	shareVersion = "0.4"

	// Persist Version Numbers
	persistVersion040 = "0.4"
	persistVersion133 = "1.3.3"
)

type (
	// persist contains all of the persistent renter data.
	persistence struct {
		MaxDownloadSpeed int64
		MaxUploadSpeed   int64
		StreamCacheSize  uint64
		Tracking         map[string]trackedFile
	}
)

// saveFile saves a file to the renter directory.
func (r *Renter) saveFile(f *siafile.SiaFile) error {
	if f.Deleted() { // TODO: violation of locking convention
		return errors.New("can't save deleted file")
	}
	// Create directory structure specified in nickname.
	fullPath := filepath.Join(r.persistDir, f.SiaPath()+ShareExtension)
	err := os.MkdirAll(filepath.Dir(fullPath), 0700)
	if err != nil {
		return err
	}

	// Open SafeFile handle.
	handle, err := persist.NewSafeFile(filepath.Join(r.persistDir, f.SiaPath()+ShareExtension))
	if err != nil {
		return err
	}
	defer handle.Close()

	// Write file data.
	err = shareFiles([]*siafile.SiaFile{f}, handle)
	if err != nil {
		return err
	}

	// Commit the SafeFile.
	return handle.CommitSync()
}

// saveSync stores the current renter data to disk and then syncs to disk.
func (r *Renter) saveSync() error {
	return persist.SaveJSON(settingsMetadata, r.persist, filepath.Join(r.persistDir, PersistFilename))
}

// loadSiaFiles walks through the directory searching for siafiles and loading
// them into memory.
func (r *Renter) loadSiaFiles() error {
	// Recursively load all files found in renter directory. Errors
	// encountered during loading are logged, but are not considered fatal.
	return filepath.Walk(r.persistDir, func(path string, info os.FileInfo, err error) error {
		// This error is non-nil if filepath.Walk couldn't stat a file or
		// folder.
		if err != nil {
			r.log.Println("WARN: could not stat file or folder during walk:", err)
			return nil
		}

		// Skip folders and non-sia files.
		if info.IsDir() || filepath.Ext(path) != ShareExtension {
			return nil
		}

		// Open the file.
		file, err := os.Open(path)
		if err != nil {
			r.log.Println("ERROR: could not open .sia file:", err)
			return nil
		}
		defer file.Close()

		// Load the file contents into the renter.
		_, err = r.loadSharedFiles(file)
		if err != nil {
			r.log.Println("ERROR: could not load .sia file:", err)
			return nil
		}
		return nil
	})
}

// load fetches the saved renter data from disk.
func (r *Renter) loadSettings() error {
	r.persist = persistence{
		Tracking: make(map[string]trackedFile),
	}
	err := persist.LoadJSON(settingsMetadata, &r.persist, filepath.Join(r.persistDir, PersistFilename))
	if os.IsNotExist(err) {
		// No persistence yet, set the defaults and continue.
		r.persist.MaxDownloadSpeed = DefaultMaxDownloadSpeed
		r.persist.MaxUploadSpeed = DefaultMaxUploadSpeed
		r.persist.StreamCacheSize = DefaultStreamCacheSize
		err = r.saveSync()
		if err != nil {
			return err
		}
	} else if err == persist.ErrBadVersion {
		// Outdated version, try the 040 to 133 upgrade.
		err = convertPersistVersionFrom040To133(filepath.Join(r.persistDir, PersistFilename))
		if err != nil {
			// Nothing left to try.
			return err
		}
		// Re-load the settings now that the file has been upgraded.
		return r.loadSettings()
	} else if err != nil {
		return err
	}

	// Set the bandwidth limits on the contractor, which was already initialized
	// without bandwidth limits.
	return r.setBandwidthLimits(r.persist.MaxDownloadSpeed, r.persist.MaxUploadSpeed)
}

// shareFiles writes the specified files to w. First a header is written,
// followed by the gzipped concatenation of each file.
func shareFiles(siaFiles []*siafile.SiaFile, w io.Writer) error {
	// Convert files to old type.
	files := make([]*file, 0, len(siaFiles))
	for _, sf := range siaFiles {
		files = append(files, siaFileToFile(sf))
	}
	// Write header.
	err := encoding.NewEncoder(w).EncodeAll(
		shareHeader,
		shareVersion,
		uint64(len(files)),
	)
	if err != nil {
		return err
	}

	// Create compressor.
	zip, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
	enc := encoding.NewEncoder(zip)

	// Encode each file.
	for _, f := range files {
		err = enc.Encode(f)
		if err != nil {
			return err
		}
	}

	return zip.Close()
}

// ShareFiles saves the specified files to shareDest.
func (r *Renter) ShareFiles(nicknames []string, shareDest string) error {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// TODO: consider just appending the proper extension.
	if filepath.Ext(shareDest) != ShareExtension {
		return ErrNonShareSuffix
	}

	handle, err := os.Create(shareDest)
	if err != nil {
		return err
	}
	defer handle.Close()

	// Load files from renter.
	files := make([]*siafile.SiaFile, len(nicknames))
	for i, name := range nicknames {
		f, exists := r.files[name]
		if !exists {
			return ErrUnknownPath
		}
		files[i] = f
	}

	err = shareFiles(files, handle)
	if err != nil {
		os.Remove(shareDest)
		return err
	}

	return nil
}

// ShareFilesASCII returns the specified files in ASCII format.
func (r *Renter) ShareFilesASCII(nicknames []string) (string, error) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// Load files from renter.
	files := make([]*siafile.SiaFile, len(nicknames))
	for i, name := range nicknames {
		f, exists := r.files[name]
		if !exists {
			return "", ErrUnknownPath
		}
		files[i] = f
	}

	buf := new(bytes.Buffer)
	err := shareFiles(files, base64.NewEncoder(base64.URLEncoding, buf))
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

	// Create decompressor.
	unzip, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	dec := encoding.NewDecoder(unzip)

	// Read each file.
	files := make([]*file, numFiles)
	for i := range files {
		files[i] = new(file)
		err := dec.Decode(files[i])
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
		r.files[f.name] = fileToSiaFile(f)
		names[i] = f.name
	}
	// Save the files.
	for _, f := range files {
		r.saveFile(fileToSiaFile(f))
	}

	return names, nil
}

// initPersist handles all of the persistence initialization, such as creating
// the persistence directory and starting the logger.
func (r *Renter) initPersist() error {
	// Create the perist directory if it does not yet exist.
	err := os.MkdirAll(r.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	r.log, err = persist.NewFileLogger(filepath.Join(r.persistDir, logFile))
	if err != nil {
		return err
	}

	// Load the prior persistence structures.
	err = r.loadSettings()
	if err != nil {
		return err
	}

	// Load the siafiles into memory.
	return r.loadSiaFiles()
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

// LoadSharedFilesASCII loads an ASCII-encoded .sia file into the renter. It
// returns the nicknames of the loaded files.
func (r *Renter) LoadSharedFilesASCII(asciiSia string) ([]string, error) {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	dec := base64.NewDecoder(base64.URLEncoding, bytes.NewBufferString(asciiSia))
	return r.loadSharedFiles(dec)
}

// convertPersistVersionFrom040to133 upgrades a legacy persist file to the next
// version, adding new fields with their default values.
func convertPersistVersionFrom040To133(path string) error {
	metadata := persist.Metadata{
		Header:  settingsMetadata.Header,
		Version: persistVersion040,
	}
	p := persistence{
		Tracking: make(map[string]trackedFile),
	}

	err := persist.LoadJSON(metadata, &p, path)
	if err != nil {
		return err
	}
	metadata.Version = persistVersion133
	p.MaxDownloadSpeed = DefaultMaxDownloadSpeed
	p.MaxUploadSpeed = DefaultMaxUploadSpeed
	p.StreamCacheSize = DefaultStreamCacheSize
	return persist.SaveJSON(metadata, p, path)
}
