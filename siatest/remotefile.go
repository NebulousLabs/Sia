package siatest

import (
	"github.com/NebulousLabs/Sia/crypto"
)

type (
	// RemoteFile is a helper struct that represents a file uploaded to the Sia
	// network.
	RemoteFile struct {
		checksum crypto.Hash
		siaPath  string
	}
)

// SiaPath returns the siaPath of a remote file.
func (rf RemoteFile) SiaPath() string {
	return rf.siaPath
}
