package siafile

import (
	"math"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	// We need to find at least erasureCode.MinPieces different pieces for each
	// chunk for the file to be available.
	for _, chunk := range sf.chunks {
		piecesForChunk := 0
		for _, pieceSet := range chunk.pieces {
			for _, piece := range pieceSet {
				if !offline[string(piece.HostPubKey.Key)] {
					piecesForChunk++
					break // break out since we only count unique pieces
				}
			}
			if piecesForChunk >= sf.erasureCode.MinPieces() {
				break // we already have enough pieces for this chunk.
			}
		}
		if piecesForChunk < sf.erasureCode.MinPieces() {
			return false // this chunk isn't available.
		}
	}
	return true
}

// ChunkSize returns the size of a single chunk of the file.
func (sf *SiaFile) ChunkSize() uint64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.chunkSize()
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

// MasterKey returns the masterkey used to encrypt the file.
func (sf *SiaFile) MasterKey() crypto.TwofishKey {
	sf.mu.RLock()
	sf.mu.RUnlock()
	return sf.metadata.masterKey
}

// Mode returns the FileMode of the SiaFile.
func (sf *SiaFile) Mode() os.FileMode {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.metadata.mode
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
	sf.mu.RLock()
	sf.mu.RUnlock()
	if sf.metadata.fileSize == 0 {
		return -1
	}

	minPiecesRenew := ^uint64(0)
	minPiecesNoRenew := ^uint64(0)
	for _, chunk := range sf.chunks {
		// Loop over chunks and remember how many unique pieces of the chunk
		// were goodForRenew and how many were not.
		numPiecesRenew := uint64(0)
		numPiecesNoRenew := uint64(0)
		for _, pieceSet := range chunk.pieces {
			// Remember if we encountered a goodForRenew piece or a
			// !goodForRenew piece that was at least online.
			foundGoodForRenew := false
			foundOnline := false
			for _, piece := range pieceSet {
				offline, exists1 := offlineMap[string(piece.HostPubKey.Key)]
				goodForRenew, exists2 := goodForRenewMap[string(piece.HostPubKey.Key)]
				if exists1 != exists2 {
					build.Critical("contract can't be in one map but not in the other")
				}
				if !exists1 || offline {
					continue
				}
				// If we found a goodForRenew piece we can stop.
				if goodForRenew {
					foundGoodForRenew = true
					break
				}
				// Otherwise we continue since there might be other hosts with
				// the same piece that are goodForRenew. We still remember that
				// we found an online piece though.
				foundOnline = true
			}
			if foundGoodForRenew {
				numPiecesRenew++
				numPiecesNoRenew++
			} else if foundOnline {
				numPiecesNoRenew++
			}
		}
		// Remember the smallest number of goodForRenew pieces encountered.
		if numPiecesRenew < minPiecesRenew {
			minPiecesRenew = numPiecesRenew
		}
		// Remember the smallest number of !goodForRenew pieces encountered.
		if numPiecesNoRenew < minPiecesNoRenew {
			minPiecesNoRenew = numPiecesNoRenew
		}
	}

	// If the redundancy is smaller than 1x we return the redundancy that
	// includes contracts that are not good for renewal. The reason for this is
	// a better user experience. If the renter operates correctly, redundancy
	// should never go above numPieces / minPieces and redundancyNoRenew should
	// never go below 1.
	redundancy := float64(minPiecesRenew) / float64(sf.erasureCode.MinPieces())
	redundancyNoRenew := float64(minPiecesNoRenew) / float64(sf.erasureCode.MinPieces())
	if redundancy < 1 {
		return redundancyNoRenew
	}
	return redundancy
}

// Rename changes the name of the file to a new one.
// TODO: This will actually rename the file on disk once we persist the new
// file format.
func (sf *SiaFile) Rename(newName string) error {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.metadata.siaPath = newName
	return nil
}

// SetMode sets the filemode of the sia file.
func (sf *SiaFile) SetMode(mode os.FileMode) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.metadata.mode = mode
}

// SiaPath returns the file's sia path.
func (sf *SiaFile) SiaPath() string {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.metadata.siaPath
}

// Size returns the file's size.
func (sf *SiaFile) Size() uint64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return uint64(sf.metadata.fileSize)
}

// UploadedBytes indicates how many bytes of the file have been uploaded via
// current file contracts. Note that this includes padding and redundancy, so
// uploadedBytes can return a value much larger than the file's original filesize.
func (sf *SiaFile) UploadedBytes() uint64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	var uploaded uint64
	for _, chunk := range sf.chunks {
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
	desired := modules.SectorSize * uint64(sf.ErasureCode().NumPieces()) * sf.NumChunks()
	return math.Min(100*(float64(uploaded)/float64(desired)), 100)
}

// ChunkSize returns the size of a single chunk of the file.
func (sf *SiaFile) chunkSize() uint64 {
	return sf.metadata.pieceSize * uint64(sf.erasureCode.MinPieces())
}
