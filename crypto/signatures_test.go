package crypto

import (
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
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
	if *pk != *unmarshalledPK {
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
	if *sig != *unmarSig {
		t.Error("signature not same after marshalling and unmarshalling")
	}
}

// TestSigning creates a bunch of keypairs and signs random data with each of
// them.
func TestSigning(t *testing.T) {
	var iterations int
	if testing.Short() {
		iterations = 5
	} else {
		iterations = 500
	}

	for i := 0; i < iterations; i++ {
		// Generate the keys.
		sk, pk, err := GenerateKeyPair()
		if err != nil {
			t.Fatal(err)
		}

		// Generate and sign the data.
		randData := make([]byte, 64)
		rand.Read(randData)
		sig, err := SignBytes(randData, sk)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the signature.
		if !VerifyBytes(randData, pk, sig) {
			t.Fatal("Signature did not verify")
		}

		// Attempt to verify after the data has been altered.
		randData[0] += 1
		if VerifyBytes(randData, pk, sig) {
			t.Fatal("Signature verified after the data was falsified")
		}

		// Attempt to verify after the signature has been altered.
		randData[0] -= 1
		sig[0] += 1
		if VerifyBytes(randData, pk, sig) {
			t.Fatal("Signature verified after the signature was altered")
		}
	}
}
