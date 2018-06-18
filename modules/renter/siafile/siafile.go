package siafile

import (
	"encoding/base32"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"

	"github.com/NebulousLabs/Sia/crypto"
)

type (
	// SiaFile is the disk format for files uploaded to the Sia network.  It
	// contains all the necessary information to recover a file from its hosts and
	// allows for easy constant-time updates of the file without having to read or
	// write the whole file.
	SiaFile struct {
		// metadata is the mostly static metadata of a SiaFile. The reserved
		// size of the metadata on disk should always be a multiple of 4kib.
		// The metadata is also the only part of the file that is JSON encoded
		// and can therefore be easily extended.
		metadata Metadata

		// pubKeyTable stores the public keys of the hosts this file's pieces are uploaded to.
		// Since multiple pieces from different chunks might be uploaded to the same host, this
		// allows us to deduplicate the rather large public keys.
		pubKeyTable []types.SiaPublicKey

		// chunks are the chunks the file was split into.
		chunks []Chunk

		// utility fields. These are not persisted.
		erasureCode modules.ErasureCoder
		mu          sync.RWMutex
		uid         string
	}

	// Chunk represents a single chunk of a file on disk
	Chunk struct {
		// erasure code settings.
		//
		// erasureCodeType specifies the algorithm used for erasure coding
		// chunks. Available types are:
		//   0 - Invalid / Missing Code
		//   1 - Reed Solomon Code
		//
		// erasureCodeParams specifies possible parameters for a certain
		// erasureCodeType. Currently params will be parsed as follows:
		//   Reed Solomon Code - 4 bytes dataPieces / 4 bytes parityPieces
		//
		erasureCodeType   [4]byte
		erasureCodeParams [8]byte

		// extensionInfo is some reserved space for each chunk that allows us
		// to indicate if a chunk is special.
		extensionInfo [16]byte

		// pieces are the pieces of the file the chunk consists of.
		// The number of pieces should equal the number of
		// dataPieces + parityPieces
		pieces []Piece
	}

	// Piece represents a single piece of a chunk on disk
	Piece struct {
		KeyNonce   [4]byte            // nonce used for encrypting the piece
		HostPubKey types.SiaPublicKey // public key of the host
		MerkleRoot crypto.Hash        // merkle root of the piece
	}
)

// New create a new SiaFile.
func New(siaPath string, erasureCode modules.ErasureCoder, masterKey crypto.TwofishKey) *SiaFile {
	file := &SiaFile{
		metadata: Metadata{
			masterKey: masterKey,
			pieceSize: modules.SectorSize - crypto.TwofishOverhead,
			siaPath:   siaPath,
		},
		erasureCode: erasureCode,
		uid:         base32.StdEncoding.EncodeToString(fastrand.Bytes(20))[:20],
	}
	return file
}

// AddPiece adds an uploaded piece to the file. It also updates the host table
// if the public key of the host is not aleady known.
func (sf *SiaFile) AddPiece(pk types.SiaPublicKey, chunkIndex, pieceIndex uint64, merkleRoot crypto.Hash) error {
	panic("Not implemented yet")
}

// ErasureCode returns the erasure coder used by the file.
func (sf *SiaFile) ErasureCode() modules.ErasureCoder {
	sf.mu.RLock()
	sf.mu.RUnlock()
	return sf.erasureCode
}

// NumChunks returns the number of chunks the file consists of.
func (sf *SiaFile) NumChunks() uint64 {
	// empty files still need at least one chunk
	if sf.metadata.fileSize == 0 {
		return 1
	}
	n := uint64(sf.metadata.fileSize) / sf.chunkSize()
	// last chunk will be padded, unless chunkSize divides file evenly.
	if uint64(sf.metadata.fileSize)%sf.chunkSize() != 0 {
		n++
	}
	return n
}

// NumPieces returns the number of pieces each chunk in the file consists of.
func (sf *SiaFile) NumPieces() uint64 {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return uint64(sf.erasureCode.NumPieces())
}

// Piece returns the piece the index pieceIndex from within the chunk at the
// index chunkIndex.
func (sf *SiaFile) Piece(chunkIndex, pieceIndex uint64) (Piece, error) {
	// TODO should return a deep copy to make sure that the caller can't modify
	// the chunks without holding a lock.
	panic("Not implemented yet")
}

// UID returns a unique identifier for this file.
func (sf *SiaFile) UID() string {
	panic("Not implemented yet")
}
