package persist

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"os"
)

var (
	// ErrBadVersion indicates that the version number of the file is not
	// compatible with the current codebase.
	ErrBadVersion = errors.New("incompatible version")

	// ErrBadHeader indicates that the file opened is not the file that was
	// expected.
	ErrBadHeader = errors.New("wrong header")
)

// Metadata contains the header and version of the data being stored.
type Metadata struct {
	Header, Version string
}

// RandomSuffix returns a 20 character base32 suffix for a filename. There are
// 100 bits of entropy, and a very low probability of colliding with existing
// files unintentionally.
func RandomSuffix() string {
	randBytes := make([]byte, 20)
	rand.Read(randBytes)
	str := base32.StdEncoding.EncodeToString(randBytes)
	return str[:20]
}

// A safeFile is a file that is stored under a temporary filename. When Commit
// is called, the file is renamed to its "final" filename. This allows for
// atomic updating of files; otherwise, an unexpected shutdown could leave a
// valuable file in a corrupted state. Callers must still Close the file handle
// as usual.
type safeFile struct {
	*os.File
	finalName string
}

// Commit renames the file to the intended final filename.
func (sf *safeFile) Commit() error {
	return os.Rename(sf.finalName+"_temp", sf.finalName)
}

// NewSafeFile returns a file that can atomically be written to disk,
// minimizing the risk of corruption.
func NewSafeFile(filename string) (*safeFile, error) {
	file, err := os.Create(filename + "_temp")
	if err != nil {
		return nil, err
	}
	return &safeFile{file, filename}, nil
}
