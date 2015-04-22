package crypto

// encrypt.go contains functions for encrypting and decrypting data byte slices
// and readers.

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/twofish"
)

var (
	ErrInsufficientLen = errors.New("supplied ciphertext is not long enough to contain a nonce")
)

type (
	Ciphertext []byte
	TwofishKey [32]byte
)

// GenerateEncryptionKey produces a key that can be used for encrypting and
// decrypting files.
func GenerateTwofishKey() (key TwofishKey, err error) {
	_, err = rand.Read(key[:])
	return
}

// EncryptBytes encrypts a []byte using the key. EncryptBytes uses GCM and
// prepends the nonce (12 bytes) to the ciphertext.
func (key TwofishKey) EncryptBytes(plaintext []byte) (ct Ciphertext, err error) {
	// Create the cipher, encryptor, and nonce.
	twofishCipher, err := twofish.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(twofishCipher)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, err
	}

	// Encrypt the data. No authenticated data is provided, as EncryptBytes is
	// meant for file encryption.
	ct = aead.Seal(nonce, nonce, plaintext, nil)
	return ct, nil
}

// DecryptBytes decrypts the ciphertext created by EncryptBytes. The nonce is
// expected to be the first 12 bytes of the ciphertext.
func (key TwofishKey) DecryptBytes(ct Ciphertext) (plaintext []byte, err error) {
	// Create the cipher.
	twofishCipher, err := twofish.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(twofishCipher)
	if err != nil {
		return nil, err
	}

	// Check for a nonce.
	if len(ct) < aead.NonceSize() {
		return nil, ErrInsufficientLen
	}

	// Decrypt the data.
	plaintext, err = aead.Open(nil, ct[:aead.NonceSize()], ct[aead.NonceSize():], nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
