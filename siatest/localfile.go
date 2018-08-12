package siatest

import (
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/fastrand"
)

type (
	// LocalFile is a helper struct that represents a file uploaded to the Sia
	// network.
	LocalFile struct {
		path     string
		checksum crypto.Hash
	}
)

// NewFile creates and returns a new LocalFile. It will write size random bytes
// to the file and give the file a random name.
func NewFile(size int) (*LocalFile, error) {
	fileName := strconv.Itoa(fastrand.Intn(math.MaxInt32))
	path := filepath.Join(SiaTestingDir, fileName)
	bytes := fastrand.Bytes(size)
	err := ioutil.WriteFile(path, bytes, 0600)
	return &LocalFile{
		path:     path,
		checksum: crypto.HashBytes(bytes),
	}, err
}

// Delete removes the LocalFile from disk.
func (lf *LocalFile) Delete() error {
	return os.Remove(lf.path)
}

// checkIntegrity compares the in-memory checksum to the checksum of the data
// on disk
func (lf *LocalFile) checkIntegrity() error {
	data, err := ioutil.ReadFile(lf.path)
	if err != nil {
		return errors.AddContext(err, "failed to read file from disk")
	}
	if crypto.HashBytes(data) != lf.checksum {
		return errors.New("checksums don't match")
	}
	return nil
}

// fileName returns the file name of the file on disk
func (lf *LocalFile) fileName() string {
	return filepath.Base(lf.path)
}

// partialChecksum returns the checksum of a part of the file.
func (lf *LocalFile) partialChecksum(from, to uint64) (crypto.Hash, error) {
	data, err := ioutil.ReadFile(lf.path)
	if err != nil {
		return crypto.Hash{}, errors.AddContext(err, "failed to read file from disk")
	}
	return crypto.HashBytes(data[from:to]), nil
}
