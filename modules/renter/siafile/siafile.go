package siafile

import (
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"reflect"
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
		deleted     bool
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
		pieces [][]Piece
	}

	// Piece represents a single piece of a chunk on disk
	Piece struct {
		KeyNonce   [4]byte            // nonce used for encrypting the piece
		HostPubKey types.SiaPublicKey // public key of the host
		MerkleRoot crypto.Hash        // merkle root of the piece
	}
)

// New create a new SiaFile.
func New(siaPath string, erasureCode modules.ErasureCoder, pieceSize, fileSize uint64) *SiaFile {
	file := &SiaFile{
		metadata: Metadata{
			fileSize:  int64(fileSize),
			masterKey: crypto.GenerateTwofishKey(),
			pieceSize: pieceSize,
			siaPath:   siaPath,
		},
		erasureCode: erasureCode,
		uid:         base32.StdEncoding.EncodeToString(fastrand.Bytes(20))[:20],
	}
	chunks := make([]Chunk, file.NumChunks())
	for i := range chunks {
		chunks[i].erasureCodeType = [4]byte{0, 0, 0, 1}
		binary.LittleEndian.PutUint32(chunks[i].erasureCodeParams[0:4], uint32(erasureCode.MinPieces()))
		binary.LittleEndian.PutUint32(chunks[i].erasureCodeParams[4:8], uint32(erasureCode.NumPieces()-erasureCode.MinPieces()))
		chunks[i].pieces = make([][]Piece, erasureCode.NumPieces())
	}
	file.chunks = chunks
	return file
}

// AddPiece adds an uploaded piece to the file. It also updates the host table
// if the public key of the host is not aleady known.
func (sf *SiaFile) AddPiece(pk types.SiaPublicKey, chunkIndex, pieceIndex uint64, merkleRoot crypto.Hash) error {
	sf.mu.Lock()
	defer sf.mu.Unlock()

	// Get the index of the host in the public key table.
	tableIndex := -1
	for i, hpk := range sf.pubKeyTable {
		if reflect.DeepEqual(hpk, pk) {
			tableIndex = i
			break
		}
	}
	// If we don't know the host yet, we add it to the table.
	if tableIndex == -1 {
		sf.pubKeyTable = append(sf.pubKeyTable, pk)
		tableIndex = len(sf.pubKeyTable) - 1
	}
	// Check if the chunkIndex is valid.
	if chunkIndex >= uint64(len(sf.chunks)) {
		return fmt.Errorf("chunkIndex %v out of bounds (%v)", chunkIndex, len(sf.chunks))
	}
	// Check if the pieceIndex is valid.
	if pieceIndex >= uint64(len(sf.chunks[chunkIndex].pieces)) {
		return fmt.Errorf("pieceIndex %v out of bounds (%v)", pieceIndex, len(sf.chunks[chunkIndex].pieces))
	}
	// Add the piece to the chunk.
	sf.chunks[chunkIndex].pieces[pieceIndex] = append(sf.chunks[chunkIndex].pieces[pieceIndex], Piece{
		HostPubKey: pk,
		MerkleRoot: merkleRoot,
	})
	return nil
}

// ErasureCode returns the erasure coder used by the file.
func (sf *SiaFile) ErasureCode() modules.ErasureCoder {
	sf.mu.RLock()
	sf.mu.RUnlock()
	return sf.erasureCode
}

// NumChunks returns the number of chunks the file consists of. This will
// return the number of chunks the file consists of even if the file is not
// fully uploaded yet.
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

// Pieces returns all the pieces for a chunk in a slice of slices that contains
// all the pieces for a certain index.
func (sf *SiaFile) Pieces(chunkIndex uint64) ([][]Piece, error) {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	if chunkIndex >= uint64(len(sf.chunks)) {
		return nil, fmt.Errorf("index %v out of bounds (%v)",
			chunkIndex, len(sf.chunks))
	}
	return sf.chunks[chunkIndex].pieces, nil
}

// UID returns a unique identifier for this file.
func (sf *SiaFile) UID() string {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	return sf.uid
}
