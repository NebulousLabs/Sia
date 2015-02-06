package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/twofish"
)

type (
	TwofishKey [32]byte
)

// Encryption and decryption of []bytes is supported, but requires keeping some
// overhead information. Also, nothing is done in-place, which means the
// functions are more memory intensive, but that's not generally the bottleneck
// when doing encryption. Developers are less likely to make mistakes when
// inputs and outputs are different memory, which is why this method has been
// chosen. Additionally, the overhead of using encryption is more transparent.

// GenerateEncryptionKey produces a key that can be used for encrypting and
// decrypting files.
func GenerateTwofishKey() (key TwofishKey, err error) {
	_, err = rand.Read(key[:])
	return
}

// EncryptBytes encrypts a []byte using the key. The padded ciphertext, iv, and
// amount of padding used are returned. `plaintext` is not overwritten.
func (key TwofishKey) EncryptBytes(plaintext []byte) (ciphertext []byte, iv []byte, padding int, err error) {
	// Determine the length needed for padding. The ciphertext must be padded
	// to a multiple of twofish.BlockSize.
	padding = twofish.BlockSize - (len(plaintext) % twofish.BlockSize)
	if padding == twofish.BlockSize {
		padding = 0
	}

	// Create the padded + unencrypted ciphertext.
	ciphertext = make([]byte, len(plaintext)+padding)
	copy(ciphertext, plaintext)

	// Create the iv.
	iv = make([]byte, twofish.BlockSize)
	_, err = rand.Read(iv)
	if err != nil {
		return
	}

	// Encrypt the ciphertext.
	blockCipher, err := twofish.NewCipher(key[:])
	if err != nil {
		return
	}
	encrypter := cipher.NewCBCEncrypter(blockCipher, iv)
	encrypter.CryptBlocks(ciphertext, ciphertext)

	return
}

// DecryptBytes decrypts a ciphertext using the key, an iv, and a volume of
// padding. `ciphertext` is not overwritten. The plaintext is returned.
func (key TwofishKey) DecryptBytes(ciphertext []byte, iv []byte, padding int) (plaintext []byte, err error) {
	// Verify the iv is the correct length.
	if len(iv) != twofish.BlockSize {
		err = errors.New("iv is not correct size")
		return
	}
	if len(ciphertext)%twofish.BlockSize != 0 {
		err = errors.New("ciphertext is not correct size")
		return
	}
	if padding > len(ciphertext) || padding < 0 {
		err = errors.New("invalid padding on ciphertext")
		return
	}

	// Allocate the plaintext.
	plaintext = make([]byte, len(ciphertext))

	// Decrypt the ciphertext.
	blockCipher, err := twofish.NewCipher(key[:])
	if err != nil {
		return
	}
	decrypter := cipher.NewCBCDecrypter(blockCipher, iv)
	decrypter.CryptBlocks(plaintext, ciphertext)

	// Remove the padding.
	plaintext = plaintext[:len(ciphertext)-padding]
	return
}
