package crypto

import (
	"crypto/rand"
	"errors"

	"github.com/NebulousLabs/ed25519"
)

const (
	EntropySize   = ed25519.EntropySize
	PublicKeySize = ed25519.PublicKeySize
	SecretKeySize = ed25519.SecretKeySize
	SignatureSize = ed25519.SignatureSize
)

type (
	PublicKey [ed25519.PublicKeySize]byte
	SecretKey [ed25519.SecretKeySize]byte
	Signature [ed25519.SignatureSize]byte
)

var (
	ErrNilInput         = errors.New("cannot use nil input")
	ErrInvalidSignature = errors.New("invalid signature")
)

// GenerateKeyPair creates a public-secret keypair that can be used to sign and
// verify messages.
func GenerateSignatureKeys() (sk SecretKey, pk PublicKey, err error) {
	var entropy [EntropySize]byte
	_, err = rand.Read(entropy[:])
	if err != nil {
		return
	}

	skPointer, pkPointer := ed25519.GenerateKey(entropy)
	return *skPointer, *pkPointer, nil
}

// DeterministicSignatureKeys generates keys deterministically using the input
// entropy. The input entropy must be 32 bytes in length.
func DeterministicSignatureKeys(entropy [EntropySize]byte) (SecretKey, PublicKey) {
	skPointer, pkPointer := ed25519.GenerateKey(entropy)
	return *skPointer, *pkPointer
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
		return ErrInvalidSignature
	}
	return nil
}

// PublicKey returns the public key that corresponds to a secret key.
func (sk SecretKey) PublicKey() (pk PublicKey) {
	copy(pk[:], sk[32:])
	return
}
