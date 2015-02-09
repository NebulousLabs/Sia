package crypto

import (
	"crypto/rand"
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

	// Call JoinHash twice on the hashes we made.
	h3 := JoinHash(h0, JoinHash(h1, h2))
	if h3 == emptyHash {
		t.Error("HashObject returned the zero hash!")
	}
}
