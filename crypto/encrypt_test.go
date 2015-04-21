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
	if bytes.Compare(plaintext, decryptedPlaintext) != 0 {
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
	if bytes.Compare(plaintext, decryptedPlaintext) != 0 {
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

// TestTwofishEntropy encrypts and then decrypts a zero plaintext, checking
// that the ciphertext is high entropy.
func TestTwofishEntropy(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Encrypt a larger zero plaintext and make sure that the outcome is high
	// entropy. Entropy is measured by compressing the ciphertext with gzip.
	// 10 * 1000 bytes was chosen to minimize the impact of gzip overhead.
	cipherSize := int(10e3)
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
	zip.Write(ciphertext)
	zip.Close()
	if b.Len() < cipherSize {
		t.Error("supposedly high entropy ciphertext has been compressed!")
	}
}
