package siatest

import (
	"bytes"
	"io/ioutil"
	"math"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/fastrand"
)

type (
	// TestFile is a helper struct to easily create files on disk that are
	// ready to use with the TestNode.
	TestFile struct {
		path     string
		fileName string
		siaPath  string
		checksum crypto.Hash
	}
)

// Compare compares the contents of two files by comparing
func (tf *TestFile) Compare(tf2 *TestFile) int {
	return bytes.Compare(tf.checksum[:], tf2.checksum[:])
}

// CompareBytes compares the contents of a TestFile to a byte slice by
// comparing its checksum to the checksum of the slice.
func (tf *TestFile) CompareBytes(data []byte) int {
	dataHash := crypto.HashObject(data)
	return bytes.Compare(tf.checksum[:], dataHash[:])
}

// updateChecksum updates the file's underlying checksum from disk.
func (tf *TestFile) updateChecksum() error {
	data, err := ioutil.ReadFile(tf.path)
	if err != nil {
		return build.ExtendErr("failed to read file", err)
	}
	tf.checksum = crypto.HashObject(data)
	return nil
}

// NewFile creates and returns a new TestFile. It will write size random bytes
// to the file and give the file a random name.
func NewFile(size int) (*TestFile, error) {
	fileName := strconv.Itoa(fastrand.Intn(math.MaxInt32))
	path := filepath.Join(SiaTestingDir, fileName)
	bytes := fastrand.Bytes(size)
	err := ioutil.WriteFile(path, bytes, 0600)
	return &TestFile{
		path:     path,
		fileName: fileName,
		siaPath:  fileName,
		checksum: crypto.HashObject(bytes),
	}, err
}
