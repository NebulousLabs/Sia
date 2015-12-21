package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"testing"
)

// TestTwofishEncryption checks that encryption and decryption works correctly.
func TestTwofishEncryption(t *testing.T) {
	// Get a key for encryption.
	key, err := GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt and decrypt a zero plaintext, and compare the decrypted to the
	// original.
	plaintext := make([]byte, 600)
	ciphertext, err := key.EncryptBytes(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decryptedPlaintext, err := key.DecryptBytes(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plaintext, decryptedPlaintext) {
		t.Fatal("Encrypted and decrypted zero plaintext do not match")
	}

	// Try again with a nonzero plaintext.
	plaintext = make([]byte, 600)
	_, err = rand.Read(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err = key.EncryptBytes(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decryptedPlaintext, err = key.DecryptBytes(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plaintext, decryptedPlaintext) {
		t.Fatal("Encrypted and decrypted zero plaintext do not match")
	}

	// Try to decrypt using a different key
	key2, err := GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}
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

	// Try to trigger a panic with nil values.
	key.EncryptBytes(nil)
	key.DecryptBytes(nil)
}

// TestReaderWriter probes the NewReader and NewWriter methods of the key type.
func TestReaderWriter(t *testing.T) {
	// Get a key for encryption.
	key, err := GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}

	// Generate plaintext.
	const plaintextSize = 600
	plaintext := make([]byte, plaintextSize)
	_, err = rand.Read(plaintext)
	if err != nil {
		t.Fatal(err)
	}

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
	key, err := GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := make([]byte, cipherSize)
	ciphertext, err := key.EncryptBytes(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Gzip the ciphertext
	var b bytes.Buffer
	zip := gzip.NewWriter(&b)
	_, err = zip.Write(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	zip.Close()
	if b.Len() < cipherSize {
		t.Error("supposedly high entropy ciphertext has been compressed!")
	}
}

// TestUnitCiphertextUnmarshalJSON tests that Ciphertext.UnmarshalJSON
// correctly fails on invalid JSON marshalled Ciphertext.
func TestUnitCiphertextUnmarshalJSON(t *testing.T) {
	// Test unmarshalling invalid JSON.
	invalidJSONBytes := [][]byte{
		nil,
		[]byte{},
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
	// Ciphertexts and corresponding JSONs to test marshalling and unmarshalling.
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

		// Marshal ct to JSON.
		jsonBytes, err := ct.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(jsonBytes, expectedJSONBytes) {
			t.Fatalf("Ciphertext %#v marshalled incorrectly: expected %q, got %q\n", ct, expectedJSONBytes, jsonBytes)
		}

		// Unmarshal back to Ciphertext.
		err = ct.UnmarshalJSON(jsonBytes)
		if err != nil {
			t.Fatal(err)
		}
		// Compare original Ciphertext (expectedCt) with resulting Ciphertext (ct).
		if expectedCt == nil && ct != nil || expectedCt != nil && ct == nil || !bytes.Equal(expectedCt, ct) {
			t.Errorf("Ciphertext %#v unmarshalled incorrectly: got %#v\n", expectedCt, ct)
		}
	}
}
