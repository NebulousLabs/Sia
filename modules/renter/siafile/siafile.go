package siafile

import (
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

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
		mu          sync.Mutex
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
		keyNonce  [4]byte     // nonce used for encrypting the piece
		pubKeyOff uint16      // offset in the pubKeyTable
		root      crypto.Hash // merkle root of the piece
	}
)
