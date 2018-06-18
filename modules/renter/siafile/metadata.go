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
		version      [16]byte          // version of the sia file format used
		fileSize     int64             // total size of the file
		masterKey    crypto.TwofishKey // masterkey used to encrypt pieces
		pieceSize    uint64            // size of a single piece of the file
		trackingPath string            // file to the local copy of the file used for repairing
		siaPath      string

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

// Available indicates whether the file is ready to be downloaded.
func (sf *SiaFile) Available(offline map[string]bool) bool {
	panic("not implemented yet")
}

// ChunkSize returns the size of a single chunk of the file.
func (sf *SiaFile) ChunkSize() uint64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.chunkSize()
}

// Delete removes the file from disk and marks it as deleted. Once the file is
// deleted, certain methods should return an error.
func (sf *SiaFile) Delete() error {
	panic("not implemented yet")
}

// Deleted indicates if this file has been deleted by the user.
func (sf *SiaFile) Deleted() bool {
	panic("not implemented yet")
}

// Expiration returns the lowest height at which any of the file's contracts
// will expire.
func (sf *SiaFile) Expiration() types.BlockHeight {
	panic("not implemented yet")
}

// HostPublicKeys returns all the public keys of hosts the file has ever been
// uploaded to. That means some of those hosts might no longer be in use.
func (sf *SiaFile) HostPublicKeys() []types.SiaPublicKey {
	panic("not implemented yet")
}

// MasterKey returns the masterkey used to encrypt the file.
func (sf *SiaFile) MasterKey() crypto.TwofishKey {
	sf.mu.RLock()
	sf.mu.RUnlock()
	return sf.metadata.masterKey
}

// Mode returns the FileMode of the SiaFile.
func (sf *SiaFile) Mode() os.FileMode {
	panic("not implemented yet")
}

// PieceSize returns the size of a single piece of the file.
func (sf *SiaFile) PieceSize() uint64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.metadata.pieceSize
}

// Redundancy returns the redundancy of the least redundant chunk. A file
// becomes available when this redundancy is >= 1. Assumes that every piece is
// unique within a file contract. -1 is returned if the file has size 0. It
// takes one argument, a map of offline contracts for this file.
func (sf *SiaFile) Redundancy(offlineMap map[string]bool, goodForRenewMap map[string]bool) float64 {
	panic("not implemented yet")
}

// Rename changes the name of the file to a new one.
func (sf *SiaFile) Rename(newName string) string {
	panic("not implemented yet")
}

// SetMode sets the filemode of the sia file.
func (sf *SiaFile) SetMode(mode os.FileMode) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.metadata.mode = mode
}

// SiaPath returns the file's sia path.
func (sf *SiaFile) SiaPath() string {
	panic("not implemented yet")
}

// Size returns the file's size.
func (sf *SiaFile) Size() uint64 {
	panic("not implemented yet")
}

// UploadedBytes indicates how many bytes of the file have been uploaded via
// current file contracts. Note that this includes padding and redundancy, so
// uploadedBytes can return a value much larger than the file's original filesize.
func (sf *SiaFile) UploadedBytes() uint64 {
	panic("not implemented yet")
}

// UploadProgress indicates what percentage of the file (plus redundancy) has
// been uploaded. Note that a file may be Available long before UploadProgress
// reaches 100%, and UploadProgress may report a value greater than 100%.
func (sf *SiaFile) UploadProgress() float64 {
	panic("not implemented yet")
}

// ChunkSize returns the size of a single chunk of the file.
func (sf *SiaFile) chunkSize() uint64 {
	return sf.metadata.pieceSize * uint64(sf.erasureCode.MinPieces())
}
