// Package dotsia defines the .sia format. It exposes functions that allow
// encoding and decoding the format.
//
// Specification
//
// A .sia file is a gzipped tar archive containing one or more Files. Each
// File is a JSON representation of the File type defined in this package. For
// each file object header in the tar archive, only the Size field is
// populated. A File only contains metadata, not the actual file as uploaded
// to the Sia network; thus, metadata pertaining to the uploaded file, such as
// its path and mode bits, are specified inside the File, not the tar header.
package dotsia

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A File contains the metadata necessary for retrieving, decoding, and
// decrypting a file stored on the Sia network.
type File struct {
	Path        string
	Size        uint64
	SectorSize  uint64
	MasterKey   crypto.TwofishKey
	Mode        os.FileMode
	ErasureCode modules.ErasureCoder
	Contracts   []Contract
}

// A Contract represents a file contract made with a host, as well as all of
// the sectors stored on that host.
type Contract struct {
	ID         types.FileContractID
	NetAddress modules.NetAddress
	EndHeight  types.BlockHeight

	Sectors []Sector
}

// A Sector refers to a sector of data stored on a host via its Merkle root.
// It also specifies the chunk and piece index of the sector, used during
// erasure coding.
type Sector struct {
	Hash  crypto.Hash
	Chunk uint64
	Piece uint64
}

// Encode writes a .sia file to w containing the supplied files.
func Encode(files []*File, w io.Writer) error {
	z := gzip.NewWriter(w)
	t := tar.NewWriter(z)

	for _, f := range files {
		encFile, err := json.Marshal(f)
		if err != nil {
			// should not be possible
			return err
		}
		err = t.WriteHeader(&tar.Header{Size: int64(len(encFile))})
		if err != nil {
			return err
		}
		_, err = t.Write(encFile)
		if err != nil {
			return err
		}
	}
	err := t.Close()
	if err != nil {
		return err
	}

	return z.Close()
}

// Decode reads a .sia file from r, returning its contents as a slice of
// Files.
func Decode(r io.Reader) ([]*File, error) {
	z, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	t := tar.NewReader(z)
	dec := json.NewDecoder(t)

	var files []*File
	for {
		_, err := t.Next()
		if err == io.EOF {
			// end of tar archive
			break
		} else if err != nil {
			return nil, err
		}
		f := new(File)
		err = dec.Decode(f)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	err = z.Close()
	if err != nil {
		return nil, err
	}

	return files, nil
}

// EncodeFile writes a .sia file to the specified filename.
func EncodeFile(files []*File, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return Encode(files, f)
}

// DecodeFile reads a .sia file from the specified filename.
func DecodeFile(filename string) ([]*File, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

// EncodeString encodes a .sia file as a base64 string.
func EncodeString(files []*File) (string, error) {
	buf := new(bytes.Buffer)
	err := Encode(files, base64.NewEncoder(base64.URLEncoding, buf))
	if err != nil {
		// should not be possible
		return "", err
	}
	return buf.String(), nil
}

// DecodeString decodes a .sia file from a base64 string.
func DecodeString(str string) ([]*File, error) {
	buf := bytes.NewBufferString(str)
	return Decode(base64.NewDecoder(base64.URLEncoding, buf))
}
