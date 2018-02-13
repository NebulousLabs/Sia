package siatest

import (
	"bytes"
	"io/ioutil"
	"math"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/fastrand"
)

type (
	// TestFile is a helper struct to easily create files on disk that are
	// ready to use with the TestNode.
	TestFile struct {
		path     string
		fileName string
		siaPath  string
	}
)

// Bytes returns the contents of the TestFile
func (tf *TestFile) Bytes() ([]byte, error) {
	return ioutil.ReadFile(tf.path)
}

// Compare is a convenience function that compares the contents of two
// TestFiles on disk. Its behavior is similar to bytes.Compare.
func (tf *TestFile) Compare(tf2 *TestFile) (int, error) {
	tfData, err := tf.Bytes()
	tf2Data, err2 := tf2.Bytes()
	if err != nil || err2 != nil {
		return 0, build.ComposeErrors(err, err2)
	}
	return bytes.Compare(tfData, tf2Data), nil
}

// CompareBytes compares the contents of a TestFile to a byte slice.
func (tf *TestFile) CompareBytes(data []byte) (int, error) {
	tfData, err := tf.Bytes()
	if err != nil {
		return 0, err
	}
	return bytes.Compare(tfData, data), nil
}

// NewFile creates and returns a new TestFile. It will write size random bytes
// to the file and give the file a random name.
func NewFile(size int) (*TestFile, error) {
	fileName := strconv.Itoa(fastrand.Intn(math.MaxInt32))
	path := filepath.Join(SiaTestingDir, fileName)
	err := ioutil.WriteFile(path, fastrand.Bytes(size), 0600)
	return &TestFile{
		path:     path,
		fileName: fileName,
		siaPath:  fileName,
	}, err
}
