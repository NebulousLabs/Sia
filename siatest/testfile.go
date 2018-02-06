package siatest

import (
	"io/ioutil"
	"math"
	"path/filepath"
	"strconv"

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
