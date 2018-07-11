package crypto

// encrypt.go contains functions for encrypting and decrypting data byte slices
// and readers.

import (
	"crypto/cipher"
	"encoding/json"
	"errors"
	"io"

	"github.com/NebulousLabs/fastrand"

	"golang.org/x/crypto/twofish"
)

const (
	// TwofishOverhead is the number of bytes added by EncryptBytes
	TwofishOverhead = 28
)

var (
	// ErrInsufficientLen is an error when supplied ciphertext is not
	// long enough to contain a nonce.
	ErrInsufficientLen = errors.New("supplied ciphertext is not long enough to contain a nonce")
)

type (
	// Ciphertext is an encrypted []byte.
	Ciphertext []byte
	// TwofishKey is a key used for encrypting and decrypting data.
	TwofishKey [EntropySize]byte
)

// GenerateTwofishKey produces a key that can be used for encrypting and
// decrypting files.
func GenerateTwofishKey() (key TwofishKey) {
	fastrand.Read(key[:])
	return
}

// NewCipher creates a new Twofish cipher from the key.
func (key TwofishKey) NewCipher() cipher.Block {
	// NOTE: NewCipher only returns an error if len(key) != 16, 24, or 32.
	cipher, _ := twofish.NewCipher(key[:])
	return cipher
}

// EncryptBytes encrypts a []byte using the key. EncryptBytes uses GCM and
// prepends the nonce (12 bytes) to the ciphertext.
func (key TwofishKey) EncryptBytes(plaintext []byte) Ciphertext {
	// Create the cipher.
	// NOTE: NewGCM only returns an error if twofishCipher.BlockSize != 16.
	aead, _ := cipher.NewGCM(key.NewCipher())

	// Create the nonce.
	nonce := fastrand.Bytes(aead.NonceSize())

	// Encrypt the data. No authenticated data is provided, as EncryptBytes is
	// meant for file encryption.
	return aead.Seal(nonce, nonce, plaintext, nil)
}

// EncryptBytesInPlace encrypts a []byte using the key. EncryptBytesInPlace
// uses GCM and prepends the nonce (12 bytes) to the ciphertext. Since it
// reuses the memory of plaintext, the input slice can't be reused after
// calling EncryptBytesInPlace.
func (key TwofishKey) EncryptBytesInPlace(plaintext []byte) Ciphertext {
	// Create the cipher.
	// NOTE: NewGCM only returns an error if twofishCipher.BlockSize != 16.
	aead, _ := cipher.NewGCM(key.NewCipher())

	// Resize the plaintext slice and free up some space at the beginning of
	// the slice.
	if !(cap(plaintext)-len(plaintext) >= aead.NonceSize()) {
		panic("capacity of plaintext too small for in-place encryption")
	}
	plaintext = plaintext[:len(plaintext)+aead.NonceSize()]
	copy(plaintext[aead.NonceSize():], plaintext)

	// Split up the plaintext slice into the nonce part and the plaintext part.
	nonce := plaintext[:aead.NonceSize()]
	plaintext = plaintext[aead.NonceSize():]

	// Create a random nonce.
	fastrand.Read(nonce)

	// Encrypt the data. No authenticated data is provided, as EncryptBytes is
	// meant for file encryption.
	return aead.Seal(nonce, nonce, plaintext, nil)
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
	nonce := ct[:aead.NonceSize()]
	ciphertext := ct[aead.NonceSize():]
	return aead.Open(nil, nonce, ciphertext, nil)
}

// DecryptBytesInPlace decrypts the ciphertext created by EncryptBytes. The
// nonce is expected to be the first 12 bytes of the ciphertext.
// DecryptBytesInPlace reuses the memory of ct to be able to operate in-place.
// This means that ct can't be reused after calling DecryptBytesInPlace.
func (key TwofishKey) DecryptBytesInPlace(ct Ciphertext) ([]byte, error) {
	// Create the cipher.
	// NOTE: NewGCM only returns an error if twofishCipher.BlockSize != 16.
	aead, _ := cipher.NewGCM(key.NewCipher())

	// Check for a nonce.
	if len(ct) < aead.NonceSize() {
		return nil, ErrInsufficientLen
	}

	// Decrypt the data.
	nonce := ct[:aead.NonceSize()]
	ciphertext := ct[aead.NonceSize():]
	return aead.Open(ciphertext[:0], nonce, ciphertext, nil)
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

// MarshalJSON returns the JSON encoding of a CipherText
func (c Ciphertext) MarshalJSON() ([]byte, error) {
	return json.Marshal([]byte(c))
}

// UnmarshalJSON parses the JSON-encoded b and returns an instance of
// CipherText.
func (c *Ciphertext) UnmarshalJSON(b []byte) error {
	var umarB []byte
	err := json.Unmarshal(b, &umarB)
	if err != nil {
		return err
	}
	*c = Ciphertext(umarB)
	return nil
}
