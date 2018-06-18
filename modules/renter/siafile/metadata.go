package siafile

import (
	"os"

	"github.com/NebulousLabs/Sia/types"
)

// Delete removes the file from disk and marks it as deleted. Once the file is
// deleted, certain methods should return an error.
func (sf *SiaFile) Delete() error {
	panic("not implemented yet")
}

// Deleted indicates if this file has been deleted by the user.
func (sf *SiaFile) Deleted() bool {
	panic("not implemented yet")
}

// HostPublicKeys returns all the public keys of hosts the file has ever been
// uploaded to. That means some of those hosts might no longer be in use.
func (sf *SiaFile) HostPublicKeys() []types.SiaPublicKey {
	panic("not implemented yet")
}

// Mode returns the FileMode of the SiaFile.
func (sf *SiaFile) Mode() os.FileMode {
	panic("not implemented yet")
}

// Name returns the file's name.
func (sf *SiaFile) Name() string {
	panic("not implemented yet")
}

// Size returns the file's size.
func (sf *SiaFile) Size() uint64 {
	panic("not implemented yet")
}
