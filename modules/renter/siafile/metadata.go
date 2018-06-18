package siafile

import (
	"os"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// Metadata is the metadata of a SiaFile and is JSON encoded.
	Metadata struct {
		version        [16]byte          // version of the sia file format used
		staticFileSize int64             // total size of the file
		masterKey      crypto.TwofishKey // masterkey used to encrypt pieces
		trackingPath   string            // file to the local copy of the file used for repairing

		// The following fields are the usual unix timestamps of files.
		modTime    time.Time // time of last content modification
		changeTime time.Time // time of last metadata modification
		accessTime time.Time // time of last access
		createTime time.Time // time of file creation

		// File ownership/permission fields.
		mode os.FileMode // unix filemode of the sia file - uint32
		uid  int         // id of the user who owns the file
		gid  int         // id of the group that owns the file

		// chunkHeaderSize is the size of each of the following chunk's metadata.
		chunkHeaderSize uint64
		// chunkBodySize is the size of each of the following chunk's bodies.
		chunkBodySize uint64

		// The following fields are the offsets for data that is written to disk
		// after the pubKeyTable. We reserve a generous amount of space for the
		// table and extra fields, but we need to remember those offsets in case we
		// need to resize later on.
		//
		// chunkOffset is the offset of the first chunk, forced to be a factor of
		// 4096, default 16kib
		//
		// pubKeyTableOffset is the office of the publicKeyTable within the
		// file.
		//
		chunkOffset       int64
		pubKeyTableOffset int64
	}
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
