package modules

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// TestAnnouncementHandling checks that CreateAnnouncement and
// DecodeAnnouncement work together correctly.
func TestAnnouncementHandling(t *testing.T) {
	t.Parallel()

	// Create the keys that will be used to generate the announcement.
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	spk := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	addr := NetAddress("foo:1234")

	// Generate the announcement.
	annBytes, err := CreateAnnouncement(addr, spk, sk)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the announcement
	ann, err := DecodeAnnouncement(annBytes)
	if err != nil {
		t.Fatal(err)
	}
	if ann.PublicKey.Algorithm != spk.Algorithm {
		t.Error("decoded announcement has the wrong algorithm on the public key")
	}
	if ann.NetAddress != addr {
		t.Error("decoded announcement has the wrong net address")
	}
	if !bytes.Equal(ann.PublicKey.Key, spk.Key) {
		t.Error("decoded announcement has the wrong public key")
	}

	// Corrupt the data, and see that decoding fails. Decoding should fail
	// because the signature should not be valid anymore.
	//
	// First 16 bytes are the host announcement prefix, followed by 8 bytes
	// describing the length of the net address, followed by the net address.
	// Corrupt the net address.
	annBytes[25]++
	_, err = DecodeAnnouncement(annBytes)
	if err != crypto.ErrInvalidSignature {
		t.Error(err)
	}
	annBytes[25]--

	// The final byte is going to be a part of the signature. Corrupt the final
	// byte and verify that there's an error.
	lastIndex := len(annBytes) - 1
	annBytes[lastIndex]++
	_, err = DecodeAnnouncement(annBytes)
	if err != crypto.ErrInvalidSignature {
		t.Error(err)
	}
	annBytes[lastIndex]--
}
