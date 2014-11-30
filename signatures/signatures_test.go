package signatures

import (
	"testing"

	"github.com/NebulousLabs/Andromeda/encoding"
)

// Creates and encodes a public key, and verifies that it decodes correctly,
// does the same with a signature.
func TestSignatureEncoding(t *testing.T) {
	// Create key pair.
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	// Marshal and unmarshal the public key.
	marshalledPK := encoding.Marshal(pk)
	var unmarshalledPK PublicKey
	err = encoding.Unmarshal(marshalledPK, &unmarshalledPK)
	if err != nil {
		t.Fatal(err)
	}

	// Test the public keys for equality.
	if pk != unmarshalledPK {
		t.Error("pubkey not the same after marshalling and unmarshalling")
	}

	// Create a signature using the secret key.
	signedData := []byte{1, 21, 31, 41, 51}
	sig, err := SignBytes(signedData, sk)
	if err != nil {
		t.Fatal(err)
	}

	// Marshal and unmarshal the signature.
	marSig := encoding.Marshal(sig)
	var unmarSig Signature
	err = encoding.Unmarshal(marSig, &unmarSig)
	if err != nil {
		t.Fatal(err)
	}

	// Test signatures for equality.
	if sig != unmarSig {
		t.Error("signature not same after marshalling and unmarshalling")
	}
}
