/*
Package dotsia defines the .sia format. It exposes functions that allow
encoding and decoding the format.

Specification

A .sia file is a gzipped tar archive containing one or more Files. Each
File is a JSON representation of the File type defined in this package. For
each file object header in the tar archive, only the Size field is
populated. A File only contains metadata, not the actual file as uploaded
to the Sia network; thus, metadata pertaining to the uploaded file, such as
its path and mode bits, are specified inside the File, not the tar header.

The first entry in the tar archive is a special metadata file. It is a JSON
object corresponding to the Metadata type exported by this package. This
object contains a version string, which indicates the version of the .sia
format used. At this time, the .sia format has no promise of backwards or
forwards compatibility, except that the version field will never be removed
from the metadata object.
*/
package dotsia

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// Header is the "magic" string that identifies a .sia file.
	Header = "Sia Shared File"

	// Version is the current version of the .sia format.
	Version = "0.6.0"
)

var (
	ErrNotSiaFile   = errors.New("not a .sia file")
	ErrIncompatible = errors.New("file is not compatible with current version")

	currentMetadata = Metadata{
		Header:  Header,
		Version: Version,
	}
)

// Metadata is the metadata entry present at the beginning of the .sia
// format's tar archive.
type Metadata struct {
	Header  string
	Version string
}

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

// writeJSONentry is a helper function that encodes a JSON object and writes
// it to tw as a complete entry.
func writeJSONentry(tw *tar.Writer, obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	err = tw.WriteHeader(&tar.Header{Size: int64(len(data))})
	if err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

// Encode writes a .sia file to w containing the supplied files.
func Encode(files []*File, w io.Writer) error {
	// Wrap w in a tar.gz writer.
	z := gzip.NewWriter(w)
	t := tar.NewWriter(z)

	// Write the metadata entry.
	err := writeJSONentry(t, currentMetadata)
	if err != nil {
		return err
	}

	// Write each file entry.
	for _, f := range files {
		err = writeJSONentry(t, f)
		if err != nil {
			return err
		}
	}

	// Close the tar archive.
	err = t.Close()
	if err != nil {
		return err
	}

	// Close the gzip writer.
	return z.Close()
}

// Decode reads a .sia file from r, returning its contents as a slice of
// Files.
func Decode(r io.Reader) ([]*File, error) {
	z, err := gzip.NewReader(r)
	if err == gzip.ErrHeader {
		return nil, ErrNotSiaFile
	} else if err != nil {
		return nil, err
	}
	t := tar.NewReader(z)
	dec := json.NewDecoder(t)

	// Read the metadata entry.
	_, err = t.Next()
	if err == io.EOF || err == tar.ErrHeader {
		// end of tar archive
		return nil, ErrNotSiaFile
	} else if err != nil {
		return nil, err
	}
	var meta Metadata
	err = dec.Decode(&meta)
	if err != nil || meta.Header != Header {
		return nil, ErrNotSiaFile
	} else if meta.Version != Version {
		return nil, ErrIncompatible
	}

	// Read the file entries
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

	// Close the gzip reader.
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
	err = Encode(files, f)
	if err != nil {
		os.Remove(filename) // clean up
		return err
	}
	return nil
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
