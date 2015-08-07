package crypto

// encrypt.go contains functions for encrypting and decrypting data byte slices
// and readers.

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"

	"golang.org/x/crypto/twofish"
)

var (
	ErrInsufficientLen = errors.New("supplied ciphertext is not long enough to contain a nonce")
)

type (
	Ciphertext []byte
	TwofishKey [EntropySize]byte
)

// GenerateEncryptionKey produces a key that can be used for encrypting and
// decrypting files.
func GenerateTwofishKey() (key TwofishKey, err error) {
	_, err = rand.Read(key[:])
	return key, err
}

// NewCipher creates a new Twofish cipher from the key.
func (key TwofishKey) NewCipher() cipher.Block {
	// NOTE: NewCipher only returns an error if len(key) != 16, 24, or 32.
	cipher, _ := twofish.NewCipher(key[:])
	return cipher
}

// EncryptBytes encrypts a []byte using the key. EncryptBytes uses GCM and
// prepends the nonce (12 bytes) to the ciphertext.
func (key TwofishKey) EncryptBytes(plaintext []byte) (Ciphertext, error) {
	// Create the cipher.
	// NOTE: NewGCM only returns an error if twofishCipher.BlockSize != 16.
	aead, _ := cipher.NewGCM(key.NewCipher())

	// Create the nonce.
	nonce, err := RandBytes(aead.NonceSize())
	if err != nil {
		return nil, err
	}

	// Encrypt the data. No authenticated data is provided, as EncryptBytes is
	// meant for file encryption.
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// DecryptBytes decrypts the ciphertext created by EncryptBytes. The nonce is
// expected to be the first 12 bytes of the ciphertext.
func (key TwofishKey) DecryptBytes(ct Ciphertext) ([]byte, error) {
	// Create the cipher.
	// NOTE: NewGCM only returns an error if twofishCipher.BlockSize != 16.
	aead, _ := cipher.NewGCM(key.NewCipher())

	// Check for a nonce.
	if len(ct) < aead.NonceSize() {
		return nil, ErrInsufficientLen
	}

	// Decrypt the data.
	return aead.Open(nil, ct[:aead.NonceSize()], ct[aead.NonceSize():], nil)
}

// NewWriter returns a writer that encrypts or decrypts its input stream.
func (key TwofishKey) NewWriter(w io.Writer) io.Writer {
	// OK to use a zero IV if the key is unique for each ciphertext.
	iv := make([]byte, twofish.BlockSize)
	stream := cipher.NewOFB(key.NewCipher(), iv)

	return &cipher.StreamWriter{S: stream, W: w}
}

// NewReader returns a reader that encrypts or decrypts its input stream.
func (key TwofishKey) NewReader(r io.Reader) io.Reader {
	// OK to use a zero IV if the key is unique for each ciphertext.
	iv := make([]byte, twofish.BlockSize)
	stream := cipher.NewOFB(key.NewCipher(), iv)

	return &cipher.StreamReader{S: stream, R: r}
}

func (c Ciphertext) MarshalJSON() ([]byte, error) {
	return json.Marshal([]byte(c))
}

func (c *Ciphertext) UnmarshalJSON(b []byte) error {
	var umarB []byte
	err := json.Unmarshal(b, &umarB)
	if err != nil {
		return err
	}
	*c = Ciphertext(umarB)
	return nil
}
