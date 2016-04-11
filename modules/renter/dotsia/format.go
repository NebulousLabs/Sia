/*
Package dotsia defines the .sia format.

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

The JSON encoding tries to be flexible in allowing arbitrary encryption and
encoding schemes. As such, the "masterKey" and "erasureCode" fields are
encoded as generic JSON objects (in Go, a map[string]interface{}). However,
these objects must always contain a "name" field that identifies the scheme
used. They may not be null.

Integer fields must not contain negative values.

The "permissions" field is encoded as a decimal number (not octal, or a
symbolic string) and must not exceed 511 (0777 in octal).

The "path" string must be an absolute Unix-style path; that is, it must begin
with a leading slash and use '/' as its separator. Additionally, paths must
not end in a slash, and must not contain any occurences of the current
directory (.) or parent directory (..) elements.

The "contracts" array must not be null, but it may be empty. The same rule
applies to the "sectors" array inside the contract object.

The "id" and "merkleRoot" fields are encoded as hex strings, which should
always be 64 bytes long.

Default values are not specified in the event that a field is undefined.
Implementations should treat such objects as malformed.

All fields use the camelCase naming convention.

Sample Data

A full example of a .sia file (omitting tar + gzip details) is show below:

	[gzip header]
	[tar header]
	{
		"header": "Sia Shared File",
		"version": "0.6.0"
	}
	[tar header]
	{
		"path": "/foo/bar/baz",
		"size": 1234567890,
		"sectorSize": 4194304,
		"masterKey": {
			"name": "twofish",
			"key": "9f775c79b9c05944b5212a855d132b7b6b9ca50ffff210d63510549e53724d6c"
		},
		"permissions": 438,
		"erasureCode": {
			"name": "reed-solomon",
			"data": 4,
			"parity": 24
		},
		"contracts": [
			{
				"id": "dbd0cc359d2f9e238cbcfcf972b18118dfb9075210c4b77d551e4285e16872a3",
				"hostAddress": "127.0.0.1:2048",
				"endHeight": 40000,
				"sectors": [
					{
						"merkleRoot": "e8926a62bcc7bb054e9f0fcc53111e26ca93067347e9386db38000159d7b7e6d",
						"chunk": 0,
						"piece": 0
					},
					{
						"merkleRoot": "1794ff6c29f7dfef3fa7a5743233b9906f34c833c053c885411cea4b18f57672",
						"chunk": 1,
						"piece": 0
					},
				]
			},
			{
				"id": "0b700dbc48ba8d6a44faf171942e32bb8326f079a376bf0e4ab2c327e368f6cb",
				"hostAddress": "192.168.1.10:1979",
				"endHeight": 50000,
				"sectors": [
					{
						"merkleRoot": "d44f12b57db08cdc1f102fdf30a80ccac11762b284e0daf12a711c6e9d001469",
						"chunk": 0,
						"piece": 1
					},
					{
						"merkleRoot": "dcfe720afcb2b12b26e1d8f821c0137c6529736b7e4a337b867940b417d53359",
						"chunk": 1,
						"piece": 1
					},
				]
			}
		]
	}
	[tar header]
	{
		"path": "/minimal_valid_file",
		"size": 0,
		"sectorSize": 0,
		"masterKey": {
			"name": ""
		},
		"permissions": 0,
		"erasureCode": {
			"name": ""
		},
		"contracts": []
	}
	[gzip footer]
*/
package dotsia

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const (
	// Header is the "magic" string that identifies a .sia file.
	Header = "Sia Shared File"

	// Version is the current version of the .sia format.
	Version = "0.6.0"
)

// These errors may be returned when Decode is called on invalid data.
var (
	ErrNotSiaFile   = errors.New("not a .sia file")
	ErrIncompatible = errors.New("file is not compatible with version " + Version)
	ErrInvalid      = errors.New("file contains invalid values")
	ErrWrongLen     = errors.New("Hex string is not 64 bytes long")
)

// Metadata is the metadata entry present at the beginning of the .sia
// format's tar archive.
type Metadata struct {
	Header  string `json:"header"`
	Version string `json:"version"`
}

var currentMetadata = Metadata{Header, Version}

// A Hash is a 32-byte checksum, encoded as a 64-byte hex string.
type Hash [32]byte

// MarshalJSON implements the json.Marshaler interface.
func (h Hash) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(h[:]))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (h *Hash) UnmarshalJSON(b []byte) error {
	var hexString string
	if err := json.Unmarshal(b, &hexString); err != nil {
		return err
	}
	if n, err := hex.Decode(h[:], []byte(hexString)); err != nil {
		return err
	} else if n != len(h) {
		return ErrWrongLen
	}
	return nil
}

// A File contains the metadata necessary for retrieving, decoding, and
// decrypting a file stored on the Sia network.
type File struct {
	Path        string                 `json:"path"`
	Size        uint64                 `json:"size"`
	SectorSize  uint64                 `json:"sectorSize"`
	Permissions os.FileMode            `json:"permissions"`
	MasterKey   map[string]interface{} `json:"masterKey"`
	ErasureCode map[string]interface{} `json:"erasureCode"`
	Contracts   []Contract             `json:"contracts"`
}

// A Contract represents a file contract made with a host, as well as all of
// the sectors stored on that host.
type Contract struct {
	ID          Hash     `json:"id"`
	HostAddress string   `json:"hostAddress"`
	EndHeight   uint64   `json:"endHeight"`
	Sectors     []Sector `json:"sectors"`
}

// A Sector refers to a sector of data stored on a host via its Merkle root.
// It also specifies the chunk and piece index of the sector, used during
// erasure coding.
type Sector struct {
	MerkleRoot Hash   `json:"merkleRoot"`
	Chunk      uint64 `json:"chunk"`
	Piece      uint64 `json:"piece"`
}

// Validate checks that f conforms to the specification defined in the package
// docstring. Files returned by Decode are automatically validated.
func (f *File) Validate() bool {
	// check path
	if !filepath.IsAbs(f.Path) || filepath.Clean(f.Path) != f.Path || f.Path == "/" {
		return false
	}
	// check permissions
	if f.Permissions > 0777 {
		return false
	}
	// check master key
	if f.MasterKey == nil || f.MasterKey["name"] == nil {
		return false
	}
	// check erasure code
	if f.ErasureCode == nil || f.ErasureCode["name"] == nil {
		return false
	}
	// check contracts
	if f.Contracts == nil {
		return false
	}
	for _, c := range f.Contracts {
		if c.Sectors == nil {
			return false
		}
	}
	return true
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
		if !f.Validate() {
			return ErrInvalid
		}
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
		} else if !f.Validate() {
			return nil, ErrInvalid
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
