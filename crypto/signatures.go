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
	ErrRandUnexpected   = errors.New("unexpected result from random number generator")
)

// KeyPairGenerator is an interface that allows the caller to generate
// public-secret key pairs.
type KeyPairGenerator interface {
	Generate() (SecretKey, PublicKey, error)
	GenerateDetermistic(entropy [EntropySize]byte) (SecretKey, PublicKey)
}

// readBytesFunc is a function pointer that reads bytes into a buffer and
// returns the total number of bytes written to the buffer.
type readBytesFunc func([]byte) (int, error)

// deriveEd25519Func is a function pointer that matches the signature of
// ed25519.GenerateKey.
type deriveEd25519Func func([EntropySize]byte) (ed25519.SecretKey, ed25519.PublicKey)

// SignatureKeyGenerator is an implementation of KeyPairGenerator.
type SignatureKeyGenerator struct {
	readRandBytes readBytesFunc
	deriveKeyPair deriveEd25519Func
}

// NewSignatureKeyGenerator creates a new SignatureKeyGenerator type that uses
// random data and depends on the ed25519.GenerateKey function for deriving
// key pairs.
func NewSignatureKeyGenerator() SignatureKeyGenerator {
	return SignatureKeyGenerator{rand.Read, ed25519.GenerateKey}
}

// Generate creates a public-secret keypair that can be used to sign and verify
// messages.
func (skg SignatureKeyGenerator) Generate() (sk SecretKey, pk PublicKey, err error) {
	var entropy [EntropySize]byte
	written, err := skg.readRandBytes(entropy[:])
	if err != nil {
		return
	}
	if written != EntropySize {
		// readRandBytes did not fill the buffer. This should never happen.
		return sk, pk, ErrRandUnexpected
	}

	skPointer, pkPointer := skg.deriveKeyPair(entropy)
	return *skPointer, *pkPointer, nil
}

// GenerateDeterministic generates keys deterministically using the input
// entropy. The input entropy must be 32 bytes in length.
func (skg SignatureKeyGenerator) GenerateDeterministic(entropy [EntropySize]byte) (SecretKey, PublicKey) {
	skPointer, pkPointer := skg.deriveKeyPair(entropy)
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
