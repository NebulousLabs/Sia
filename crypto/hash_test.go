package crypto

import (
	"crypto/rand"
	"encoding/json"
	"sort"
	"testing"
)

// TestObject is a struct that's used for testing HashAll and HashObject. The
// fields have to be exported so the encoder can read them.
type TestObject struct {
	A int
	B byte
	C bool
	D string
}

// testHashing uses each of the functions in hash.go and verifies that the
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
	data := make([]byte, 435)
	rand.Read(data)
	h2 := HashBytes(data)
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
