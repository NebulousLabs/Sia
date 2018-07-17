package crypto

import (
	"bytes"
	"compress/gzip"
	"testing"

	"gitlab.com/NebulousLabs/fastrand"
)

// TestTwofishEncryption checks that encryption and decryption works correctly.
func TestTwofishEncryption(t *testing.T) {
	// Get a key for encryption.
	key := GenerateTwofishKey()

	// Encrypt and decrypt a zero plaintext, and compare the decrypted to the
	// original.
	plaintext := make([]byte, 600)
	ciphertext := key.EncryptBytes(plaintext)
	decryptedPlaintext, err := key.DecryptBytes(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plaintext, decryptedPlaintext) {
		t.Fatal("Encrypted and decrypted zero plaintext do not match")
	}

	// Try again with a nonzero plaintext.
	plaintext = fastrand.Bytes(600)
	ciphertext = key.EncryptBytes(plaintext)
	decryptedPlaintext, err = key.DecryptBytes(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plaintext, decryptedPlaintext) {
		t.Fatal("Encrypted and decrypted zero plaintext do not match")
	}

	// Try to decrypt using a different key
	key2 := GenerateTwofishKey()
	_, err = key2.DecryptBytes(ciphertext)
	if err == nil {
		t.Fatal("Expecting failed authentication err", err)
	}

	// Try to decrypt using bad ciphertexts.
	ciphertext[0]++
	_, err = key.DecryptBytes(ciphertext)
	if err == nil {
		t.Fatal("Expecting failed authentication err", err)
	}
	_, err = key.DecryptBytes(ciphertext[:10])
	if err != ErrInsufficientLen {
		t.Error("Expecting ErrInsufficientLen:", err)
	}

	// Try to trigger a panic or error with nil values.
	key.EncryptBytes(nil)
	_, err = key.DecryptBytes(nil)
	if err != ErrInsufficientLen {
		t.Error("Expecting ErrInsufficientLen:", err)
	}
}

// TestReaderWriter probes the NewReader and NewWriter methods of the key type.
func TestReaderWriter(t *testing.T) {
	// Get a key for encryption.
	key := GenerateTwofishKey()

	// Generate plaintext.
	const plaintextSize = 600
	plaintext := fastrand.Bytes(plaintextSize)

	// Create writer and encrypt plaintext.
	buf := new(bytes.Buffer)
	key.NewWriter(buf).Write(plaintext)

	// There should be no overhead present.
	if buf.Len() != plaintextSize {
		t.Fatalf("encryption introduced %v bytes of overhead", buf.Len()-plaintextSize)
	}

	// Create reader and decrypt ciphertext.
	var decrypted = make([]byte, plaintextSize)
	key.NewReader(buf).Read(decrypted)

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("couldn't decrypt encrypted stream")
	}
}

// TestTwofishEntropy encrypts and then decrypts a zero plaintext, checking
// that the ciphertext is high entropy.
func TestTwofishEntropy(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Encrypt a larger zero plaintext and make sure that the outcome is high
	// entropy. Entropy is measured by compressing the ciphertext with gzip.
	// 10 * 1000 bytes was chosen to minimize the impact of gzip overhead.
	const cipherSize = 10e3
	key := GenerateTwofishKey()
	plaintext := make([]byte, cipherSize)
	ciphertext := key.EncryptBytes(plaintext)

	// Gzip the ciphertext
	var b bytes.Buffer
	zip := gzip.NewWriter(&b)
	_, err := zip.Write(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	zip.Close()
	if b.Len() < cipherSize {
		t.Error("supposedly high entropy ciphertext has been compressed!")
	}
}

// TestUnitCiphertextUnmarshalInvalidJSON tests that Ciphertext.UnmarshalJSON
// correctly fails on invalid JSON marshalled Ciphertext.
func TestUnitCiphertextUnmarshalInvalidJSON(t *testing.T) {
	// Test unmarshalling invalid JSON.
	invalidJSONBytes := [][]byte{
		nil,
		{},
		[]byte("\""),
	}
	for _, jsonBytes := range invalidJSONBytes {
		var ct Ciphertext
		err := ct.UnmarshalJSON(jsonBytes)
		if err == nil {
			t.Errorf("expected unmarshall to fail on the invalid JSON: %q\n", jsonBytes)
		}
	}
}

// TestCiphertextMarshalling tests that marshalling Ciphertexts to JSON results
// in the expected JSON. Also tests that marshalling that JSON back to
// Ciphertext results in the original Ciphertext.
func TestCiphertextMarshalling(t *testing.T) {
	// Ciphertexts and corresponding JSONs to test marshalling and
	// unmarshalling.
	ciphertextMarshallingTests := []struct {
		ct        Ciphertext
		jsonBytes []byte
	}{
		{ct: Ciphertext(nil), jsonBytes: []byte("null")},
		{ct: Ciphertext(""), jsonBytes: []byte(`""`)},
		{ct: Ciphertext("a ciphertext"), jsonBytes: []byte(`"YSBjaXBoZXJ0ZXh0"`) /* base64 encoding of the Ciphertext */},
	}
	for _, test := range ciphertextMarshallingTests {
		expectedCt := test.ct
		expectedJSONBytes := test.jsonBytes

		// Create a copy of expectedCt so Unmarshalling does not modify it, as
		// we need it later for comparison.
		var ct Ciphertext
		if expectedCt == nil {
			ct = nil
		} else {
			ct = make(Ciphertext, len(expectedCt))
			copy(ct, expectedCt)
		}

		// Marshal Ciphertext to JSON.
		jsonBytes, err := ct.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(jsonBytes, expectedJSONBytes) {
			// Use %#v instead of %v because %v prints Ciphertexts constructed
			// with nil and []byte{} identically.
			t.Fatalf("Ciphertext %#v marshalled incorrectly: expected %q, got %q\n", ct, expectedJSONBytes, jsonBytes)
		}

		// Unmarshal back to Ciphertext.
		err = ct.UnmarshalJSON(jsonBytes)
		if err != nil {
			t.Fatal(err)
		}
		// Compare resulting Ciphertext with expected Ciphertext.
		if expectedCt == nil && ct != nil || expectedCt != nil && ct == nil || !bytes.Equal(expectedCt, ct) {
			// Use %#v instead of %v because %v prints Ciphertexts constructed
			// with nil and []byte{} identically.
			t.Errorf("Ciphertext %#v unmarshalled incorrectly: got %#v\n", expectedCt, ct)
		}
	}
}

// TestTwofishNewCipherAssumption tests that the length of a TwofishKey is 16,
// 24, or 32 as these are the only cases where twofish.NewCipher(key[:])
// doesn't return an error.
func TestTwofishNewCipherAssumption(t *testing.T) {
	// Generate key.
	key := GenerateTwofishKey()
	// Test key length.
	keyLen := len(key)
	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		t.Errorf("TwofishKey must have length 16, 24, or 32, but generated key has length %d\n", keyLen)
	}
}

// TestCipherNewGCMAssumption tests that the BlockSize of a cipher block is 16,
// as this is the only case where cipher.NewGCM(block) doesn't return an error.
func TestCipherNewGCMAssumption(t *testing.T) {
	// Generate a key and then cipher block from key.
	key := GenerateTwofishKey()
	// Test block size.
	block := key.NewCipher()
	if block.BlockSize() != 16 {
		t.Errorf("cipher must have BlockSize 16, but generated cipher has BlockSize %d\n", block.BlockSize())
	}
}
