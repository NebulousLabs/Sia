package siatest

import (
	"errors"
	"io/ioutil"
	"math"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
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
		checksum: crypto.HashObject(bytes),
	}, err
}

// checkIntegrity compares the in-memory checksum to the checksum of the data
// on disk
func (lf *LocalFile) checkIntegrity() error {
	data, err := ioutil.ReadFile(lf.path)
	if err != nil {
		return build.ExtendErr("failed to read file from disk", err)
	}
	if crypto.HashAll(data) != lf.checksum {
		return errors.New("checksums don't match")
	}
	return nil
}

// fileName returns the file name of the file on disk
func (lf *LocalFile) fileName() string {
	return filepath.Base(lf.path)
}
