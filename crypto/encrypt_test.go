package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"testing"
)

// Test encryption makes sure that things can be encrypted and decrypted, and
// that they at least appear random.
//
// CONTRIBUTE: Additional tests could be used, such as testing that decryption
// fails if the wrong iv's are used, and overall trying to probe the library
// for something that doesn't work quite right.
func TestEncryption(t *testing.T) {
	// Get a key for encryption.
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt the zero plaintext.
	zeroPlaintext := make([]byte, 128)
	ciphertext, iv, padding, err := EncryptBytes(key, zeroPlaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Get the decrypted plaintext.
	decryptedZeroPlaintext, err := DecryptBytes(key, ciphertext, iv, padding)
	if err != nil {
		t.Fatal(err)
	}

	// Compare the original to the decrypted.
	if bytes.Compare(zeroPlaintext, decryptedZeroPlaintext) != 0 {
		t.Fatal("Encrypted and decrypted zero plaintext do not match")
	}

	if testing.Short() {
		t.Skip()
	}

	// Encrypt and decrypt for all of the potential padded values and see that
	// padding is handled correctly.
	for i := 256; i < 256+BlockSize; i++ {
		key, err := GenerateEncryptionKey()
		if err != nil {
			t.Fatal(err)
		}
		plaintext := make([]byte, i)
		rand.Read(plaintext)
		ciphertext, iv, padding, err := EncryptBytes(key, plaintext)
		if err != nil {
			t.Fatal(err)
		}
		decryptedPlaintext, err := DecryptBytes(key, ciphertext, iv, padding)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(plaintext, decryptedPlaintext) != 0 {
			t.Fatal("Encrypted and decrypted zero plaintext do not match for i = ", i)
		}
	}

	// Encrypt a larger zero plaintext and make sure that the outcome is high
	// entropy. We measure entropy by seeing how much gzip can compress the
	// ciphertext. 10 * 1000 bytes was chosen because gzip overhead will exceed
	// compression rate for smaller files, even low entropy files.
	cipherSize := 10 * 1000
	key, err = GenerateEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := make([]byte, cipherSize)
	ciphertext, iv, padding, err = EncryptBytes(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Gzip the ciphertext
	var b bytes.Buffer
	zip := gzip.NewWriter(&b)
	zip.Write(ciphertext)
	zip.Close()
	if b.Len() < cipherSize {
		t.Error("high entropy ciphertext is compressing!")
	}
}
