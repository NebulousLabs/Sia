package crypto

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/NebulousLabs/fastrand"
)

type (
	// TestObject is a struct that's used for testing HashAll and HashObject. The
	// fields have to be exported so the encoder can read them.
	TestObject struct {
		A int
		B byte
		C bool
		D string
	}
)

// TestHashing uses each of the functions in hash.go and verifies that the
// results are as expected.
func TestHashing(t *testing.T) {
	// Create a test object.
	to := TestObject{
		A: 12345,
		B: 5,
		C: true,
		D: "testing",
	}

	// Call HashObject on the object.
	var emptyHash Hash
	h0 := HashObject(to)
	if h0 == emptyHash {
		t.Error("HashObject returned the zero hash!")
	}

	// Call HashAll on the test object and some other fields.
	h1 := HashAll(
		int(122),
		byte(115),
		string("test"),
		to,
	)
	if h1 == emptyHash {
		t.Error("HashObject returned the zero hash!")
	}

	// Call HashBytes on a random byte slice.
	h2 := HashBytes(fastrand.Bytes(435))
	if h2 == emptyHash {
		t.Error("HashObject returned the zero hash!")
	}
}

// TestHashSorting takes a set of hashses and checks that they can be sorted.
func TestHashSorting(t *testing.T) {
	// Created an unsorted list of hashes.
	hashes := make([]Hash, 5)
	hashes[0][0] = 12
	hashes[1][0] = 7
	hashes[2][0] = 13
	hashes[3][0] = 14
	hashes[4][0] = 1

	// Sort the hashes.
	sort.Sort(HashSlice(hashes))
	if hashes[0][0] != 1 {
		t.Error("bad sort")
	}
	if hashes[1][0] != 7 {
		t.Error("bad sort")
	}
	if hashes[2][0] != 12 {
		t.Error("bad sort")
	}
	if hashes[3][0] != 13 {
		t.Error("bad sort")
	}
	if hashes[4][0] != 14 {
		t.Error("bad sort")
	}
}

// TestUnitHashMarshalJSON tests that Hashes are correctly marshalled to JSON.
func TestUnitHashMarshalJSON(t *testing.T) {
	h := HashObject("an object")
	jsonBytes, err := h.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jsonBytes, []byte(`"`+h.String()+`"`)) {
		t.Errorf("hash %s encoded incorrectly: got %s\n", h, jsonBytes)
	}
}

// TestUnitHashUnmarshalJSON tests that unmarshalling invalid JSON will result
// in an error.
func TestUnitHashUnmarshalJSON(t *testing.T) {
	// Test unmarshalling invalid data.
	invalidJSONBytes := [][]byte{
		// Invalid JSON.
		nil,
		{},
		[]byte("\""),
		// JSON of wrong length.
		[]byte(""),
		[]byte(`"` + strings.Repeat("a", HashSize*2-1) + `"`),
		[]byte(`"` + strings.Repeat("a", HashSize*2+1) + `"`),
		// JSON of right length but invalid Hashes.
		[]byte(`"` + strings.Repeat("z", HashSize*2) + `"`),
		[]byte(`"` + strings.Repeat(".", HashSize*2) + `"`),
		[]byte(`"` + strings.Repeat("\n", HashSize*2) + `"`),
	}

	for _, jsonBytes := range invalidJSONBytes {
		var h Hash
		err := h.UnmarshalJSON(jsonBytes)
		if err == nil {
			t.Errorf("expected unmarshall to fail on the invalid JSON: %q\n", jsonBytes)
		}
	}

	// Test unmarshalling valid data.
	expectedH := HashObject("an object")
	jsonBytes := []byte(`"` + expectedH.String() + `"`)

	var h Hash
	err := h.UnmarshalJSON(jsonBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(h[:], expectedH[:]) {
		t.Errorf("Hash %s marshalled incorrectly: got %s\n", expectedH, h)
	}
}

// TestHashMarshalling checks that the marshalling of the hash type works as
// expected.
func TestHashMarshalling(t *testing.T) {
	h := HashObject("an object")
	hBytes, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}

	var uMarH Hash
	err = uMarH.UnmarshalJSON(hBytes)
	if err != nil {
		t.Fatal(err)
	}

	if h != uMarH {
		t.Error("encoded and decoded hash do not match!")
	}
}

// TestHashLoadString checks that the LoadString method of the hash function is
// working properly.
func TestHashLoadString(t *testing.T) {
	h1 := Hash{}
	h2 := HashObject("tame")
	h1e := h1.String()
	h2e := h2.String()

	var h1d, h2d Hash
	err := h1d.LoadString(h1e)
	if err != nil {
		t.Fatal(err)
	}
	err = h2d.LoadString(h2e)
	if err != nil {
		t.Fatal(err)
	}
	if h1d != h1 {
		t.Error("decoding h1 failed")
	}
	if h2d != h2 {
		t.Error("decoding h2 failed")
	}

	// Try some bogus strings.
	h1e = h1e + "a"
	err = h1.LoadString(h1e)
	if err == nil {
		t.Fatal("expecting error when decoding hash of too large length")
	}
	h1e = h1e[:60]
	err = h1.LoadString(h1e)
	if err == nil {
		t.Fatal("expecting error when decoding hash of too small length")
	}
}
