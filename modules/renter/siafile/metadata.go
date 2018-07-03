package siafile

import (
	"math"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// Metadata is the metadata of a SiaFile and is JSON encoded.
	Metadata struct {
		staticVersion   [16]byte          // version of the sia file format used
		staticFileSize  int64             // total size of the file
		staticMasterKey crypto.TwofishKey // masterkey used to encrypt pieces
		staticPieceSize uint64            // size of a single piece of the file
		localPath       string            // file to the local copy of the file used for repairing
		siaPath         string            // the path of the file on the Sia network

		// The following fields are the usual unix timestamps of files.
		modTime    time.Time // time of last content modification
		changeTime time.Time // time of last metadata modification
		accessTime time.Time // time of last access
		createTime time.Time // time of file creation

		// File ownership/permission fields.
		mode os.FileMode // unix filemode of the sia file - uint32
		uid  int         // id of the user who owns the file
		gid  int         // id of the group that owns the file

		// staticChunkMetadataSize is the amount of space allocated within the
		// siafile for the metadata of a single chunk. It allows us to do
		// random access operations on the file in constant time.
		staticChunkMetadataSize uint64

		// The following fields are the offsets for data that is written to disk
		// after the pubKeyTable. We reserve a generous amount of space for the
		// table and extra fields, but we need to remember those offsets in case we
		// need to resize later on.
		//
		// chunkOffset is the offset of the first chunk, forced to be a factor of
		// 4096, default 4kib
		//
		// pubKeyTableOffset is the offset of the publicKeyTable within the
		// file.
		//
		chunkOffset       int64
		pubKeyTableOffset int64
	}
)

// ChunkSize returns the size of a single chunk of the file.
func (sf *SiaFile) ChunkSize(chunkIndex uint64) uint64 {
	return sf.staticChunkSize(chunkIndex)
}

// Delete removes the file from disk and marks it as deleted. Once the file is
// deleted, certain methods should return an error.
func (sf *SiaFile) Delete() {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.deleted = true
}

// Deleted indicates if this file has been deleted by the user.
func (sf *SiaFile) Deleted() bool {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.deleted
}

// Expiration returns the lowest height at which any of the file's contracts
// will expire.
func (sf *SiaFile) Expiration(contracts map[string]modules.RenterContract) types.BlockHeight {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	if len(sf.pubKeyTable) == 0 {
		return 0
	}

	lowest := ^types.BlockHeight(0)
	for _, pk := range sf.pubKeyTable {
		contract, exists := contracts[string(pk.Key)]
		if !exists {
			continue
		}
		if contract.EndHeight < lowest {
			lowest = contract.EndHeight
		}
	}
	return lowest
}

// HostPublicKeys returns all the public keys of hosts the file has ever been
// uploaded to. That means some of those hosts might no longer be in use.
func (sf *SiaFile) HostPublicKeys() []types.SiaPublicKey {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.pubKeyTable
}

// LocalPath returns the path of the local data of the file.
func (sf *SiaFile) LocalPath() string {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.staticMetadata.localPath
}

// MasterKey returns the masterkey used to encrypt the file.
func (sf *SiaFile) MasterKey() crypto.TwofishKey {
	return sf.staticMetadata.staticMasterKey
}

// Mode returns the FileMode of the SiaFile.
func (sf *SiaFile) Mode() os.FileMode {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.staticMetadata.mode
}

// PieceSize returns the size of a single piece of the file.
func (sf *SiaFile) PieceSize() uint64 {
	return sf.staticMetadata.staticPieceSize
}

// Rename changes the name of the file to a new one.
// TODO: This will actually rename the file on disk once we persist the new
// file format.
func (sf *SiaFile) Rename(newName string) error {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.staticMetadata.siaPath = newName
	return nil
}

// SetMode sets the filemode of the sia file.
func (sf *SiaFile) SetMode(mode os.FileMode) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.staticMetadata.mode = mode
}

// SetLocalPath changes the local path of the file which is used to repair
// the file from disk.
func (sf *SiaFile) SetLocalPath(path string) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.staticMetadata.localPath = path
}

// SiaPath returns the file's sia path.
func (sf *SiaFile) SiaPath() string {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.staticMetadata.siaPath
}

// Size returns the file's size.
func (sf *SiaFile) Size() uint64 {
	return uint64(sf.staticMetadata.staticFileSize)
}

// UploadedBytes indicates how many bytes of the file have been uploaded via
// current file contracts. Note that this includes padding and redundancy, so
// uploadedBytes can return a value much larger than the file's original filesize.
func (sf *SiaFile) UploadedBytes() uint64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	var uploaded uint64
	for _, chunk := range sf.staticChunks {
		for _, pieceSet := range chunk.pieces {
			// Note: we need to multiply by SectorSize here instead of
			// f.pieceSize because the actual bytes uploaded include overhead
			// from TwoFish encryption
			uploaded += uint64(len(pieceSet)) * modules.SectorSize
		}
	}
	return uploaded
}

// UploadProgress indicates what percentage of the file (plus redundancy) has
// been uploaded. Note that a file may be Available long before UploadProgress
// reaches 100%, and UploadProgress may report a value greater than 100%.
func (sf *SiaFile) UploadProgress() float64 {
	uploaded := sf.UploadedBytes()
	var desired uint64
	for i := uint64(0); i < sf.NumChunks(); i++ {
		desired += modules.SectorSize * uint64(sf.ErasureCode(i).NumPieces())
	}
	return math.Min(100*(float64(uploaded)/float64(desired)), 100)
}

// ChunkSize returns the size of a single chunk of the file.
func (sf *SiaFile) staticChunkSize(chunkIndex uint64) uint64 {
	return sf.staticMetadata.staticPieceSize * uint64(sf.staticChunks[chunkIndex].staticErasureCode.MinPieces())
}
