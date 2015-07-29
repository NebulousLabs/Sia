package persist

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
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
