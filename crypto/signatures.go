package crypto

import (
	"errors"

	"github.com/NebulousLabs/ed25519"
)

const (
	// EntropySize defines the amount of entropy necessary to do secure
	// cryptographic operations, in bytes.
	EntropySize = ed25519.EntropySize

	// PublicKeySize defines the size of public keys in bytes.
	PublicKeySize = ed25519.PublicKeySize

	// SecretKeySize defines the size of secret keys in bytes.
	SecretKeySize = ed25519.SecretKeySize

	// SignatureSize defines the size of signatures in bytes.
	SignatureSize = ed25519.SignatureSize
)

type (
	// PublicKey is an object that can be used to verify signatures.
	PublicKey [ed25519.PublicKeySize]byte

	// SecretKey can be used to sign data for the corresponding public key.
	SecretKey [ed25519.SecretKeySize]byte

	// Signature proves that data was signed by the owner of a particular
	// public key's corresponding secret key.
	Signature [ed25519.SignatureSize]byte
)

var (
	// errInvalidSignature is returned if a signature is provided that does not
	// match the data and public key.
	errInvalidSignature = errors.New("invalid signature")
)

// GenerateKeyPair creates a public-secret keypair that can be used to sign and verify
// messages.
func GenerateKeyPair() (sk SecretKey, pk PublicKey, err error) {
	return stdKeyGen.generate()
}

// GenerateKeyPairDeterministic generates keys deterministically using the input
// entropy. The input entropy must be 32 bytes in length.
func GenerateKeyPairDeterministic(entropy [EntropySize]byte) (SecretKey, PublicKey) {
	return stdKeyGen.generateDeterministic(entropy)
}

// SignHash signs a message using a secret key.
func SignHash(data Hash, sk SecretKey) (sig Signature, err error) {
	skNorm := [SecretKeySize]byte(sk)
	sig = *ed25519.Sign(&skNorm, data[:])
	return sig, nil
}

// VerifyHash uses a public key and input data to verify a signature.
func VerifyHash(data Hash, pk PublicKey, sig Signature) error {
	pkNorm := [PublicKeySize]byte(pk)
	sigNorm := [SignatureSize]byte(sig)
	verifies := ed25519.Verify(&pkNorm, data[:], &sigNorm)
	if !verifies {
		return errInvalidSignature
	}
	return nil
}

// PublicKey returns the public key that corresponds to a secret key.
func (sk SecretKey) PublicKey() (pk PublicKey) {
	copy(pk[:], sk[32:])
	return
}
