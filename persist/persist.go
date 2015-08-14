package persist

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"os"
)

var (
	ErrBadVersion = errors.New("incompatible version")
	ErrBadHeader  = errors.New("wrong header")
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

// A safeFile is a file that is stored under a temporary filename. When the
// safeFile is closed, it will be renamed to the "final" filename. This allows
// for atomic updating of files; otherwise, an unexpected shutdown could leave
// a valuable file in a corrupted state.
type safeFile struct {
	*os.File
	finalName string
}

// Close renames the file to the intended final filename, and then closes the
// file handle.
func (sf *safeFile) Close() error {
	err := os.Rename(sf.finalName+"_temp", sf.finalName)
	if err != nil {
		return err
	}
	return sf.File.Close()
}

func NewSafeFile(filename string) (*safeFile, error) {
	file, err := os.Create(filename + "_temp")
	if err != nil {
		return nil, err
	}
	return &safeFile{file, filename}, nil
}
