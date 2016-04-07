package dotsia

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

type mockWriter func([]byte) (int, error)

func (fn mockWriter) Write(p []byte) (int, error) {
	return fn(p)
}

type mockReader func([]byte) (int, error)

func (fn mockReader) Read(p []byte) (int, error) {
	return fn(p)
}

// TestEncodeDecode tests the Encode and Decode functions, which are inverses
// of each other.
func TestEncodeDecode(t *testing.T) {
	buf := new(bytes.Buffer)
	fs := make([]*File, 100)
	for i := range fs {
		fs[i] = &File{
			Size:       uint64(i),
			Mode:       os.FileMode(i),
			SectorSize: uint64(i),
		}
	}
	err := Encode(fs, buf)
	if err != nil {
		t.Fatal(err)
	}
	savedBuf := buf.String() // used later
	files, err := Decode(buf)
	if err != nil {
		t.Fatal(err)
	}
	// verify that files were not changed after encode/decode
	for i := range files {
		if files[i].Size != fs[i].Size ||
			files[i].Mode != fs[i].Mode ||
			files[i].SectorSize != fs[i].SectorSize {
			t.Errorf("File %d differs after encoding: %v %v", i, files[i], fs[i])
		}
	}

	// try decoding invalid data
	b := []byte(savedBuf)
	b[0] = 0xFF
	_, err = Decode(bytes.NewReader(b))
	if err != ErrNotSiaFile {
		t.Fatal("expected header error, got", err)
	}
	b = []byte(savedBuf)
	b[500] = 0xFF
	_, err = Decode(bytes.NewReader(b))
	if _, ok := err.(*json.SyntaxError); !ok {
		t.Fatal("expected syntax error, got", err)
	}
	// empty archive
	buf.Reset()
	z := gzip.NewWriter(buf)
	tw := tar.NewWriter(z)
	err = tw.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = z.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decode(buf)
	if err != ErrNotSiaFile {
		t.Fatal(err)
	}

	// use a mockWriter to simulate write errors
	w := mockWriter(func([]byte) (int, error) {
		return 0, os.ErrInvalid
	})
	err = Encode(fs, w)
	if err != os.ErrInvalid {
		t.Fatal("expected mocked error, got", err)
	}

	// use a mockReader to simulate read errors
	r := mockReader(func([]byte) (int, error) {
		return 0, os.ErrInvalid
	})
	_, err = Decode(r)
	if err != os.ErrInvalid {
		t.Fatal("expected mocked error, got", err)
	}
}

// TestEncodeDecodeFile tests the EncodeFile and DecodeFile functions, which
// are inverses of each other.
func TestEncodeDecodeFile(t *testing.T) {
	fs := make([]*File, 100)
	for i := range fs {
		fs[i] = &File{
			Size:       uint64(i),
			Mode:       os.FileMode(i),
			SectorSize: uint64(i),
		}
	}
	dir := build.TempDir("dotsia")
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, "TestEncodeDecodeFile")
	err = EncodeFile(fs, filename)
	if err != nil {
		t.Fatal(err)
	}
	files, err := DecodeFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	// verify that files were not changed after encode/decode
	for i := range files {
		if files[i].Size != fs[i].Size ||
			files[i].Mode != fs[i].Mode ||
			files[i].SectorSize != fs[i].SectorSize {
			t.Errorf("File %d differs after encoding: %v %v", i, files[i], fs[i])
		}
	}

	// make the file unreadable
	err = os.Chmod(filename, 0000)
	if err != nil {
		t.Fatal(err)
	}
	err = EncodeFile(nil, filename)
	if !os.IsPermission(err) {
		t.Fatal("expected permissions error, got", err)
	}
	_, err = DecodeFile(filename)
	if !os.IsPermission(err) {
		t.Fatal("expected permissions error, got", err)
	}
}

// TestEncodeDecodeString tests the EncodeString and DecodeString functions, which
// are inverses of each other.
func TestEncodeDecodeString(t *testing.T) {
	fs := make([]*File, 100)
	for i := range fs {
		fs[i] = &File{
			Size:       uint64(i),
			Mode:       os.FileMode(i),
			SectorSize: uint64(i),
		}
	}
	str, err := EncodeString(fs)
	if err != nil {
		t.Fatal(err)
	}
	files, err := DecodeString(str)
	if err != nil {
		t.Fatal(err)
	}
	// verify that files were not changed after encode/decode
	for i := range files {
		if files[i].Size != fs[i].Size ||
			files[i].Mode != fs[i].Mode ||
			files[i].SectorSize != fs[i].SectorSize {
			t.Errorf("File %d differs after encoding: %v %v", i, files[i], fs[i])
		}
	}
}

// TestMetadata tests the metadata validation of the Decode function.
func TestMetadata(t *testing.T) {
	// save global metadata var
	oldMeta := currentMetadata
	defer func() {
		currentMetadata = oldMeta
	}()

	// bad version
	currentMetadata.Version = "foo"
	str, err := EncodeString([]*File{new(File)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeString(str)
	if err != ErrIncompatible {
		t.Fatal("expected version error, got", err)
	}

	// bad header
	currentMetadata.Header = "foo"
	str, err = EncodeString([]*File{new(File)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeString(str)
	if err != ErrNotSiaFile {
		t.Fatal("expected header error, got", err)
	}
}
