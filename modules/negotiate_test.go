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
	sk, pk := crypto.GenerateKeyPair()
	spk := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	addr := NetAddress("f.o:1234")

	// Generate the announcement.
	annBytes, err := CreateAnnouncement(addr, spk, sk)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the announcement
	decAddr, decPubKey, err := DecodeAnnouncement(annBytes)
	if err != nil {
		t.Fatal(err)
	}
	if decPubKey.Algorithm != spk.Algorithm {
		t.Error("decoded announcement has the wrong algorithm on the public key")
	}
	if decAddr != addr {
		t.Error("decoded announcement has the wrong net address")
	}
	if !bytes.Equal(decPubKey.Key, spk.Key) {
		t.Error("decoded announcement has the wrong public key")
	}

	// Corrupt the data, and see that decoding fails. Decoding should fail
	// because the signature should not be valid anymore.
	//
	// First 16 bytes are the host announcement prefix, followed by 8 bytes
	// describing the length of the net address, followed by the net address.
	// Corrupt the net address.
	annBytes[25]++
	_, _, err = DecodeAnnouncement(annBytes)
	if err != crypto.ErrInvalidSignature {
		t.Error(err)
	}
	annBytes[25]--

	// The final byte is going to be a part of the signature. Corrupt the final
	// byte and verify that there's an error.
	lastIndex := len(annBytes) - 1
	annBytes[lastIndex]++
	_, _, err = DecodeAnnouncement(annBytes)
	if err != crypto.ErrInvalidSignature {
		t.Error(err)
	}
	annBytes[lastIndex]--

	// Pass in a bad specifier - change the host announcement type.
	annBytes[0]++
	_, _, err = DecodeAnnouncement(annBytes)
	if err != ErrAnnNotAnnouncement {
		t.Error(err)
	}
	annBytes[0]--

	// Pass in a bad signature algorithm. 16 bytes to pass the specifier, 8+8 bytes to pass the net address.
	annBytes[33]++
	_, _, err = DecodeAnnouncement(annBytes)
	if err != ErrAnnUnrecognizedSignature {
		t.Error(err)
	}
	annBytes[33]--

	// Cause the decoding to fail altogether.
	_, _, err = DecodeAnnouncement(annBytes[:12])
	if err == nil {
		t.Error(err)
	}
}

// TestNegotiationResponses tests the WriteNegotiationAcceptance,
// WriteNegotiationRejection, and ReadNegotiationAcceptance functions.
func TestNegotiationResponses(t *testing.T) {
	// Write/Read acceptance
	buf := new(bytes.Buffer)
	err := WriteNegotiationAcceptance(buf)
	if err != nil {
		t.Fatal(err)
	}
	err = ReadNegotiationAcceptance(buf)
	if err != nil {
		t.Fatal(err)
	}

	// Write/Read rejection
	buf.Reset()
	err = WriteNegotiationRejection(buf, ErrLowBalance)
	if err != ErrLowBalance {
		t.Fatal(err)
	}
	err = ReadNegotiationAcceptance(buf)
	// can't compare to ErrLowBalance directly; contents are the same, but pointer is different
	if err == nil || err.Error() != ErrLowBalance.Error() {
		t.Fatal(err)
	}

	// Write/Read StopResponse
	buf.Reset()
	err = WriteNegotiationStop(buf)
	if err != nil {
		t.Fatal(err)
	}
	err = ReadNegotiationAcceptance(buf)
	if err != ErrStopResponse {
		t.Fatal(err)
	}
}
