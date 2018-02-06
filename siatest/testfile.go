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
func (tf *TestFile) Bytes() []byte {
	data, err := ioutil.ReadFile(tf.path)
	if err != nil {
		println(err)
		return []byte{}
	}
	return data
}

// Compare is a convenience function that compares the contents of two
// TestFiles on disk. Its behavior is similar to bytes.Compare.
func (tf *TestFile) Compare(tf2 *TestFile) int {
	tfData, err := ioutil.ReadFile(tf.path)
	tf2Data, err2 := ioutil.ReadFile(tf2.path)
	if err != nil || err2 != nil {
		// Print error and return a mismatch instead of returning the error.
		// This should be sufficient for a testing environment and makes for
		// cleaner tests.
		println(build.ComposeErrors(err, err2))
		return math.MaxInt32
	}
	return bytes.Compare(tfData, tf2Data)
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
