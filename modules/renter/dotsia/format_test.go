package dotsia

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

type mockWriter func([]byte) (int, error)

func (fn mockWriter) Write(p []byte) (int, error) {
	return fn(p)
}

type mockReader func([]byte) (int, error)

func (fn mockReader) Read(p []byte) (int, error) {
	return fn(p)
}

// makeRandomFile generates a random File, containing up to 3 contracts, each
// with up to 7 sectors. On average, this seems to produce about 300 bytes of
// entropy.
func makeRandomFile() *File {
	entropy, err := crypto.RandBytes(6)
	if err != nil {
		panic(err)
	}
	contracts := make([]Contract, entropy[0]&0x03)
	for i := range contracts {
		sectors := make([]Sector, entropy[1]&0x07)
		for j := range sectors {
			sectors[j] = Sector{
				Chunk: uint64(entropy[j%len(entropy)]),
				Piece: uint64(entropy[i%len(entropy)]),
			}
			rand.Read(sectors[j].MerkleRoot[:])
		}
		contracts[i] = Contract{
			EndHeight: uint64(entropy[2]),
			Sectors:   sectors,
		}
		rand.Read(contracts[i].ID[:])
	}

	return &File{
		Path:        "random/file",
		Size:        uint64(entropy[3]),
		Permissions: os.FileMode(entropy[4]),
		SectorSize:  uint64(entropy[5]),
		MasterKey:   map[string]interface{}{"name": "random-key"},
		ErasureCode: map[string]interface{}{"name": "random-code"},
		Contracts:   contracts,
	}
}

// TestHashMarshalling tests the MarshalJSON and UnmarshalJSON methods of the
// Hash type.
func TestHashMarshalling(t *testing.T) {
	h := Hash{1, 2, 3}
	jsonBytes, err := h.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jsonBytes, []byte(`"`+hex.EncodeToString(h[:])+`"`)) {
		t.Errorf("hash %s encoded incorrectly: got %s\n", h, jsonBytes)
	}

	var h2 Hash
	err = h2.UnmarshalJSON(jsonBytes)
	if err != nil {
		t.Fatal(err)
	} else if h != h2 {
		t.Error("encoded and decoded hash do not match!")
	}

	quote := func(b []byte) []byte {
		return append([]byte{'"'}, append(b, '"')...)
	}

	// Test unmarshalling invalid data.
	invalidJSONBytes := [][]byte{
		// Invalid JSON.
		nil,
		{},
		[]byte(`"`),
		// JSON of wrong length.
		[]byte(""),
		quote(bytes.Repeat([]byte{'a'}, len(h))),
		quote(bytes.Repeat([]byte{'a'}, len(h)*2+1)),
		// JSON of right length but invalid Hashes.
		quote(bytes.Repeat([]byte{'z'}, len(h)*2)),
		quote(bytes.Repeat([]byte{'.'}, len(h)*2)),
		quote(bytes.Repeat([]byte{'\n'}, len(h)*2)),
	}

	for _, jsonBytes := range invalidJSONBytes {
		var h Hash
		err := h.UnmarshalJSON(jsonBytes)
		if err == nil {
			t.Errorf("expected unmarshal to fail on the invalid JSON: %q\n", jsonBytes)
		}
	}
}

// TestMarshalParity tests that the Hash type encodes identically to the
// types.FileContractID and crypto.Hash types, and that the modules.NetAddress
// type encodes identically to a string.
func TestMarshalParity(t *testing.T) {
	var randBytes [32]byte
	rand.Read(randBytes[:])

	hashTests := [][32]byte{
		{},
		{1},
		{1, 2, 3},
		randBytes,
	}
	for _, h := range hashTests {
		hashBytes, hashErr := json.Marshal(Hash(h))
		cHashBytes, cHashErr := json.Marshal(crypto.Hash(h))
		idBytes, idErr := json.Marshal(types.FileContractID(h))
		if hashErr != nil || cHashErr != nil || idErr != nil {
			t.Error("encoding error:", hashErr, cHashErr, idErr)
		} else if !bytes.Equal(hashBytes, idBytes) {
			t.Error("encoded Hash does not match types.FileContractID:", hashBytes, idBytes)
		} else if !bytes.Equal(hashBytes, cHashBytes) {
			t.Error("encoded Hash does not match crypto.Hash:", hashBytes, cHashBytes)
		}
	}

	addrTests := []string{
		"",
		"foo",
		"foo:1234",
		"foo:",
		"foo\n\t",
		"::1",
		"[::]:1234",
	}
	for _, str := range addrTests {
		strBytes, strErr := json.Marshal(str)
		netAddrBytes, naErr := json.Marshal(modules.NetAddress(str))
		if strErr != nil || naErr != nil {
			t.Error("encoding error:", strErr, naErr)
		} else if !bytes.Equal(strBytes, netAddrBytes) {
			t.Error("encoded string does not match modules.NetAddress:", strBytes, netAddrBytes)
		}
	}
}

// TestValidate tests that the Validate method correctly checks each
// validation requirement.
func TestValidate(t *testing.T) {
	// Minimum valid file
	minimumValid := File{
		Path:        "test",
		Permissions: 0,
		MasterKey:   map[string]interface{}{"name": ""},
		ErasureCode: map[string]interface{}{"name": ""},
		Contracts:   []Contract{},
	}
	if !minimumValid.Validate() {
		t.Error("minimum valid file should be valid")
	}

	// Path
	pathTests := []struct {
		path string
		ok   bool
	}{
		{"/absolute/path", false},         // absolute path
		{"unclean///path///", false},      // not a clean path
		{"directory/../traversal", false}, // not a clean path
		{"folder/", false},                // not a clean path
		{"./foo", false},                  // not a clean path
		{".", false},                      // empty
		{"/", false},                      // absolute path

		{"foo", true},         // normal file
		{"foo/bar/baz", true}, // normal file
		{".foo", true},        // dotfile
	}
	for i, test := range pathTests {
		f := minimumValid
		f.Path = test.path
		if f.Validate() != test.ok {
			t.Errorf("test %v failed: Validate(Path: %v) should be %v", i, test.path, test.ok)
		}
	}

	// Permissions
	permTest := minimumValid
	permTest.Permissions = 01000
	if permTest.Validate() {
		t.Error("file with perm > 0777 should be invalid")
	}

	// MasterKey
	keyTest := minimumValid
	keyTest.MasterKey = nil
	if keyTest.Validate() {
		t.Error("file with nil MasterKey should be invalid")
	}
	keyTest.MasterKey = map[string]interface{}{"foo": "bar"}
	if keyTest.Validate() {
		t.Error("file with MasterKey missing name field should be invalid")
	}

	// ErasureCode
	codeTest := minimumValid
	codeTest.ErasureCode = nil
	if codeTest.Validate() {
		t.Error("file with nil ErasureCode should be invalid")
	}
	codeTest.ErasureCode = map[string]interface{}{"foo": "bar"}
	if codeTest.Validate() {
		t.Error("file with ErasureCode missing name field should be invalid")
	}

	// Contracts
	contractTest := minimumValid
	contractTest.Contracts = nil
	if contractTest.Validate() {
		t.Error("file with nil Contracts should be invalid")
	}
	contractTest.Contracts = []Contract{{}}
	if contractTest.Validate() {
		t.Error("file with nil Sector should be invalid")
	}
}

// TestEncodeDecode tests the Encode and Decode functions, which are inverses
// of each other.
func TestEncodeDecode(t *testing.T) {
	buf := new(bytes.Buffer)
	fs := make([]*File, 100)
	for i := range fs {
		fs[i] = makeRandomFile()
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
			files[i].Permissions != fs[i].Permissions ||
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
		fs[i] = makeRandomFile()
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
			files[i].Permissions != fs[i].Permissions ||
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
		fs[i] = makeRandomFile()
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
			files[i].Permissions != fs[i].Permissions ||
			files[i].SectorSize != fs[i].SectorSize {
			t.Errorf("File %d differs after encoding: %v %v", i, files[i], fs[i])
		}
	}

	// try encoding invalid File
	_, err = EncodeString([]*File{new(File)})
	if err != ErrInvalid {
		t.Error("expected ErrInvalid, got", err)
	}
}

// TestMetadata tests the metadata validation of the Decode function.
func TestMetadata(t *testing.T) {
	// save global metadata var
	oldMeta := currentMetadata
	defer func() {
		currentMetadata = oldMeta
	}()

	// Minimum valid file
	minimumValid := &File{
		Path:        "test",
		Permissions: 0,
		MasterKey:   map[string]interface{}{"name": ""},
		ErasureCode: map[string]interface{}{"name": ""},
		Contracts:   []Contract{},
	}

	// bad version
	currentMetadata.Version = "foo"
	str, err := EncodeString([]*File{minimumValid})
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeString(str)
	if err != ErrIncompatible {
		t.Fatal("expected version error, got", err)
	}

	// bad header
	currentMetadata.Header = "foo"
	str, err = EncodeString([]*File{minimumValid})
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeString(str)
	if err != ErrNotSiaFile {
		t.Fatal("expected header error, got", err)
	}
}

// TestEncodedSize checks that the size of a .sia file is within reasonable
// bounds.
func TestEncodedSize(t *testing.T) {
	// generate 100 random files
	fs := make([]*File, 100)
	for i := range fs {
		fs[i] = makeRandomFile()
	}

	buf := new(bytes.Buffer)
	err := Encode(fs, buf)
	if err != nil {
		t.Fatal(err)
	}

	// should be no more than ~500 bytes of entropy per file
	maxSize := len(fs) * 500
	minSize := len(fs) * 100
	if size := buf.Len(); size > maxSize {
		t.Fatalf(".sia file is too large: max is %v, got %v", maxSize, size)
	} else if size < minSize {
		t.Fatalf(".sia file is too small: min is %v, got %v", minSize, size)
	}
}

// BenchmarkEncode benchmarks the Encode function.
func BenchmarkEncode(b *testing.B) {
	// generate 100 random files
	fs := make([]*File, 100)
	for i := range fs {
		fs[i] = makeRandomFile()
	}

	// to get an accurate number of bytes processed, we need to know the
	// length before tarring + gzipping
	data, err := json.Marshal(fs)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))

	buf := new(bytes.Buffer)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		err := Encode(fs, buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecode benchmarks the Decode function.
func BenchmarkDecode(b *testing.B) {
	// generate 100 random files
	fs := make([]*File, 100)
	for i := range fs {
		fs[i] = makeRandomFile()
	}
	// write to buffer
	buf := new(bytes.Buffer)
	err := Encode(fs, buf)
	if err != nil {
		b.Fatal(err)
	}

	// to get an accurate number of bytes processed, we need to know the
	// length before tarring + gzipping
	data, err := json.Marshal(fs)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))

	r := bytes.NewReader(buf.Bytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		_, err = Decode(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}
