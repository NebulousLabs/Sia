package siafile

import (
	"encoding/binary"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

type (
	// FileData is a helper struct that contains all the relevant information
	// of a file. It simplifies passing the necessary data between modules and
	// keeps the interface clean.
	FileData struct {
		Name        string
		FileSize    uint64
		MasterKey   crypto.TwofishKey
		ErasureCode modules.ErasureCoder
		PieceSize   uint64
		Mode        os.FileMode
		Deleted     bool
		UID         string
		Chunks      []FileChunk
	}
	// FileChunk is a helper struct that contains data about a chunk.
	FileChunk struct {
		Pieces [][]Piece
	}
)

// NewFromFileData creates a new SiaFile from a FileData object that was
// previously created from a legacy file.
func NewFromFileData(fd FileData) *SiaFile {
	file := &SiaFile{
		metadata: Metadata{
			fileSize:  int64(fd.FileSize),
			masterKey: fd.MasterKey,
			mode:      fd.Mode,
			pieceSize: fd.PieceSize,
			siaPath:   fd.Name,
		},
		deleted:     fd.Deleted,
		erasureCode: fd.ErasureCode,
		uid:         fd.UID,
	}
	chunks := make([]Chunk, file.NumChunks())
	for i := range chunks {
		chunks[i].erasureCodeType = [4]byte{0, 0, 0, 1}
		binary.LittleEndian.PutUint32(chunks[i].erasureCodeParams[0:4], uint32(file.erasureCode.MinPieces()))
		binary.LittleEndian.PutUint32(chunks[i].erasureCodeParams[4:8], uint32(file.erasureCode.NumPieces()-file.erasureCode.MinPieces()))
		chunks[i].pieces = make([][]Piece, file.erasureCode.NumPieces())
	}
	file.chunks = chunks

	// Populate the pubKeyTable of the file and add the pieces.
	pubKeyMap := make(map[string]int)
	for chunkIndex, chunk := range fd.Chunks {
		for pieceIndex, pieceSet := range chunk.Pieces {
			for _, piece := range pieceSet {
				// Check if we already added that public key.
				if _, exists := pubKeyMap[string(piece.HostPubKey.Key)]; !exists {
					pubKeyMap[string(piece.HostPubKey.Key)] = len(file.pubKeyTable)
					file.pubKeyTable = append(file.pubKeyTable, piece.HostPubKey)
				}
				// Add the piece to the SiaFile.
				file.chunks[chunkIndex].pieces[pieceIndex] = append(file.chunks[chunkIndex].pieces[pieceIndex], Piece{
					HostPubKey: piece.HostPubKey,
					MerkleRoot: piece.MerkleRoot,
				})
			}
		}
	}
	return file
}

// ExportFileData creates a FileData object from a SiaFile that can be used to
// convert the file into a legacy file.
func (sf *SiaFile) ExportFileData() FileData {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	fd := FileData{
		Name:        sf.metadata.siaPath,
		FileSize:    uint64(sf.metadata.fileSize),
		MasterKey:   sf.metadata.masterKey,
		ErasureCode: sf.erasureCode,
		PieceSize:   sf.metadata.pieceSize,
		Mode:        sf.metadata.mode,
		Deleted:     sf.deleted,
		UID:         sf.uid,
	}
	// Return a deep-copy to avoid race conditions.
	fd.Chunks = make([]FileChunk, len(sf.chunks))
	for chunkIndex := range fd.Chunks {
		fd.Chunks[chunkIndex].Pieces = make([][]Piece, len(sf.chunks[chunkIndex].pieces))
		for pieceIndex := range fd.Chunks[chunkIndex].Pieces {
			fd.Chunks[chunkIndex].Pieces[pieceIndex] = make([]Piece, len(sf.chunks[chunkIndex].pieces[pieceIndex]))
			copy(fd.Chunks[chunkIndex].Pieces[pieceIndex], sf.chunks[chunkIndex].pieces[pieceIndex])
		}
	}
	return fd
}
