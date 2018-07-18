package crypto

import (
	"bytes"
	"testing"

	"gitlab.com/NebulousLabs/Sia/encoding"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestUnitSignatureEncoding creates and encodes a public key, and verifies
// that it decodes correctly, does the same with a signature.
func TestUnitSignatureEncoding(t *testing.T) {
	// Create a dummy key pair.
	var sk SecretKey
	sk[0] = 4
	sk[32] = 5
	pk := sk.PublicKey()

	// Marshal and unmarshal the public key.
	marshalledPK := encoding.Marshal(pk)
	var unmarshalledPK PublicKey
	err := encoding.Unmarshal(marshalledPK, &unmarshalledPK)
	if err != nil {
		t.Fatal(err)
	}

	// Test the public keys for equality.
	if pk != unmarshalledPK {
		t.Error("pubkey not the same after marshalling and unmarshalling")
	}

	// Create a signature using the secret key.
	var signedData Hash
	fastrand.Read(signedData[:])
	sig := SignHash(signedData, sk)

	// Marshal and unmarshal the signature.
	marshalledSig := encoding.Marshal(sig)
	var unmarshalledSig Signature
	err = encoding.Unmarshal(marshalledSig, &unmarshalledSig)
	if err != nil {
		t.Fatal(err)
	}

	// Test signatures for equality.
	if sig != unmarshalledSig {
		t.Error("signature not same after marshalling and unmarshalling")
	}

}

// TestUnitSigning creates a bunch of keypairs and signs random data with each of
// them.
func TestUnitSigning(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Try a bunch of signatures because at one point there was a library that
	// worked around 98% of the time. Tests would usually pass, but 200
	// iterations would normally cause a failure.
	iterations := 200
	for i := 0; i < iterations; i++ {
		// Create dummy key pair.
		sk, pk := GenerateKeyPair()

		// Generate and sign the data.
		var randData Hash
		fastrand.Read(randData[:])
		sig := SignHash(randData, sk)

		// Verify the signature.
		err := VerifyHash(randData, pk, sig)
		if err != nil {
			t.Fatal(err)
		}

		// Attempt to verify after the data has been altered.
		randData[0]++
		err = VerifyHash(randData, pk, sig)
		if err != ErrInvalidSignature {
			t.Fatal(err)
		}

		// Restore the data and make sure the signature is valid again.
		randData[0]--
		err = VerifyHash(randData, pk, sig)
		if err != nil {
			t.Fatal(err)
		}

		// Attempt to verify after the signature has been altered.
		sig[0]++
		err = VerifyHash(randData, pk, sig)
		if err != ErrInvalidSignature {
			t.Fatal(err)
		}
	}
}

// TestIntegrationSigKeyGenerate is an integration test checking that
// GenerateKeyPair and GenerateKeyPairDeterminisitc accurately create keys.
func TestIntegrationSigKeyGeneration(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	message := HashBytes([]byte{'m', 's', 'g'})

	// Create a random key and use it.
	randSecKey, randPubKey := GenerateKeyPair()
	sig := SignHash(message, randSecKey)
	err := VerifyHash(message, randPubKey, sig)
	if err != nil {
		t.Error(err)
	}
	// Corrupt the signature
	sig[0]++
	err = VerifyHash(message, randPubKey, sig)
	if err == nil {
		t.Error("corruption failed")
	}

	// Create a deterministic key and use it.
	var detEntropy [EntropySize]byte
	detEntropy[0] = 35
	detSecKey, detPubKey := GenerateKeyPairDeterministic(detEntropy)
	sig = SignHash(message, detSecKey)
	err = VerifyHash(message, detPubKey, sig)
	if err != nil {
		t.Error(err)
	}
	// Corrupt the signature
	sig[0]++
	err = VerifyHash(message, detPubKey, sig)
	if err == nil {
		t.Error("corruption failed")
	}
}

// TestReadWriteSignedObject tests the ReadSignObject and WriteSignedObject
// functions, which are inverses of each other.
func TestReadWriteSignedObject(t *testing.T) {
	sk, pk := GenerateKeyPair()

	// Write signed object into buffer.
	b := new(bytes.Buffer)
	err := WriteSignedObject(b, "foo", sk)
	if err != nil {
		t.Fatal(err)
	}
	// Keep a copy of b's bytes.
	buf := b.Bytes()

	// Read and verify object.
	var read string
	err = ReadSignedObject(b, &read, 11, pk)
	if err != nil {
		t.Fatal(err)
	}
	if read != "foo" {
		t.Fatal("encode/decode mismatch: expected 'foo', got", []byte(read))
	}

	// Check that maxlen is being respected.
	b = bytes.NewBuffer(buf) // reset b
	err = ReadSignedObject(b, &read, 10, pk)
	if err == nil || err.Error() != "length 11 exceeds maxLen of 10" {
		t.Fatal("expected length error, got", err)
	}

	// Disrupt the decoding to get coverage on the failure branch.
	err = ReadSignedObject(b, &read, 11, pk)
	if err == nil || err.Error() != "could not decode type crypto.Signature: unexpected EOF" {
		t.Fatal(err)
	}

	// Try with an invalid signature.
	buf[0]++                 // alter the first byte of the signature, invalidating it.
	b = bytes.NewBuffer(buf) // reset b
	err = ReadSignedObject(b, &read, 11, pk)
	if err != ErrInvalidSignature {
		t.Fatal(err)
	}
}

// TestUnitPublicKey tests the PublicKey method
func TestUnitPublicKey(t *testing.T) {
	for i := 0; i < 1000; i++ {
		sk, pk := GenerateKeyPair()
		if sk.PublicKey() != pk {
			t.Error("PublicKey does not match actual public key:", pk, sk.PublicKey())
		}
	}
}
