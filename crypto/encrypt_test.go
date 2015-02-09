package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/twofish"
)

// TestTwofishEncryption checks that encryption and decryption works correctly.
func TestTwofishEncryption(t *testing.T) {
	// Get a key for encryption.
	key, err := GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt the zero plaintext.
	plaintext := make([]byte, 128)
	_, err = rand.Read(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, iv, padding, err := key.EncryptBytes(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Get the decrypted plaintext.
	decryptedPlaintext, err := key.DecryptBytes(ciphertext, iv, padding)
	if err != nil {
		t.Fatal(err)
	}

	// Compare the original to the decrypted.
	if bytes.Compare(plaintext, decryptedPlaintext) != 0 {
		t.Fatal("Encrypted and decrypted zero plaintext do not match")
	}

	// Try to decrypt using a different key
	key2, err := GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}
	badtext, err := key2.DecryptBytes(ciphertext, iv, padding)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(plaintext, badtext) == 0 {
		t.Fatal("When using the wrong key, plaintext was still decrypted!")
	}

	// Try to decrypt using a different iv.
	badIV := iv
	badIV[0]++
	badtext, err = key.DecryptBytes(ciphertext, badIV, padding)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(plaintext, badtext) == 0 {
		t.Fatal("When using the wrong key, plaintext was still decrypted!")
	}

	// Try to decrypt using incorrectly sized ivs, ciphertext, and padding.
	_, err = key.DecryptBytes(ciphertext, iv[1:], padding)
	if err == nil {
		t.Fatal("Was able to decrypt with a bad iv.")
	}
	_, err = key.DecryptBytes(ciphertext[1:], iv, padding)
	if err == nil {
		t.Fatal("Was able to decrypt with a bad ciphertext")
	}
	_, err = key.DecryptBytes(ciphertext, iv, 1+len(ciphertext))
	if err == nil {
		t.Fatal("Was able to decrypt using bad padding")
	}
	_, err = key.DecryptBytes(ciphertext, iv, -1)
	if err == nil {
		t.Fatal("Was able to decrypt using bad padding")
	}

	// Try to trigger a panic with nil values.
	key.EncryptBytes(nil)
	key.DecryptBytes(nil, iv, padding)
	key.DecryptBytes(ciphertext, nil, padding)

}

// TestTwofishEntropy encrypts and then decrypts a zero plaintext, checking
// that the ciphertext is high entropy. This is simply to check for obvious
// mistakes and not to guarantee security of the ciphertext.
func TestTwofishEntropy(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Encrypt a larger zero plaintext and make sure that the outcome is high
	// entropy. We measure entropy by seeing how much gzip can compress the
	// ciphertext. 10 * 1000 bytes was chosen because gzip overhead will exceed
	// compression rate for smaller files, even low entropy files.
	cipherSize := int(10e3) // default is a float? - caused an error in `make([]byte, cipherSize)`
	key, err := GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := make([]byte, cipherSize)
	ciphertext, _, _, err := key.EncryptBytes(plaintext)
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

// TestTwofishPadding encrypts and decrypts a byte slice that invokes every
// possible padding length.
func TestTwofishPadding(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Encrypt and decrypt for all of the potential padded values and see that
	// padding is handled correctly.
	for i := 256; i < 256+twofish.BlockSize; i++ {
		key, err := GenerateTwofishKey()
		if err != nil {
			t.Fatal(err)
		}
		plaintext := make([]byte, i)
		rand.Read(plaintext)
		ciphertext, iv, padding, err := key.EncryptBytes(plaintext)
		if err != nil {
			t.Fatal(err)
		}
		decryptedPlaintext, err := key.DecryptBytes(ciphertext, iv, padding)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Compare(plaintext, decryptedPlaintext) != 0 {
			t.Fatal("Encrypted and decrypted zero plaintext do not match for i = ", i)
		}
	}
}
