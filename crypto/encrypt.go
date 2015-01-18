package crypto

import (
	"crypto/cipher"
	"crypto/rand"

	"golang.org/x/crypto/twofish"
)

const (
	BlockSize = twofish.BlockSize
	KeySize   = 32
)

type (
	EncryptionKey [KeySize]byte
)

// Encryption and decryption of []bytes is supported, but requires keeping some
// overhead information. Also, nothing is done in-place, which means the
// functions are more memory intensive, but that's not generally the bottleneck
// when doing encryption. Developers are less likely to make mistakes when
// inputs and outputs are different memory, which is why this method has been
// chosen. Additionally, the overhead of using encryption is more transparent.

// GenerateEncryptionKey produces a key that can be used for encrypting and
// decrypting files.
func GenerateEncryptionKey() (key EncryptionKey, err error) {
	_, err = rand.Read(key[:])
	if err != nil {
		return
	}

	return
}

// EncryptBytes encrypts a []byte using a key. The padded ciphertext, iv, and
// amount of padding used are returned. `plaintext` is not overwritten.
func EncryptBytes(key EncryptionKey, plaintext []byte) (ciphertext []byte, iv []byte, padding int, err error) {
	// Determine the length needed for padding. The ciphertext must be padded
	// to a multiple of BlockSize.
	padding = BlockSize - (len(plaintext) % BlockSize)
	if padding == BlockSize {
		padding = 0
	}

	// Create the padded + unencrypted ciphertext.
	ciphertext = make([]byte, len(plaintext)+padding)
	copy(ciphertext, plaintext)

	// Create the iv.
	iv = make([]byte, BlockSize)
	_, err = rand.Read(iv)
	if err != nil {
		return
	}

	// Encrypt the ciphertext.
	block, err := twofish.NewCipher(key[:])
	if err != nil {
		return
	}
	encrypter := cipher.NewCBCEncrypter(block, iv)
	encrypter.CryptBlocks(ciphertext, ciphertext)

	return
}

// DecryptBytes decrypts a ciphertext using a key, an iv, and a volume of
// padding. `ciphertext` is not overwritten. The plaintext is returned.
func DecryptBytes(key EncryptionKey, ciphertext []byte, iv []byte, padding int) (plaintext []byte, err error) {
	// Verify the iv is the correct length.
	if len(iv) != BlockSize {
		return errors.New("iv is not correct size")
	}
	if len(ciphertext) % BlockSize != 0 {
		return errors.New("ciphertext is not correct size")
	}
	if padding > ciphertext {
		return errors.New("stated padding is longer than the ciphertext")
	}

	// Allocate the plaintext.
	plaintext = make([]byte, len(ciphertext))

	// Decrypt the ciphertext.
	block, err := twofish.NewCipher(key[:])
	if err != nil {
		return
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext)

	// Remove the padding.
	plaintext = plaintext[:len(ciphertext)-padding]
	return
}
