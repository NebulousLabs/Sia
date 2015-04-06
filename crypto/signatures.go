package crypto

import (
	"crypto/rand"
	"errors"

	"github.com/agl/ed25519"
)

const (
	PublicKeySize = ed25519.PublicKeySize
	SecretKeySize = ed25519.PrivateKeySize
	SignatureSize = ed25519.SignatureSize
)

type (
	PublicKey [ed25519.PublicKeySize]byte
	SecretKey [ed25519.PrivateKeySize]byte
	Signature [ed25519.SignatureSize]byte
)

var (
	ErrNilInput         = errors.New("cannot use nil input")
	ErrInvalidSignature = errors.New("invalid signature")
)

// GenerateKeyPair creates a public-secret keypair that can be used to sign and
// verify messages.
func GenerateSignatureKeys() (sk SecretKey, pk PublicKey, err error) {
	pkPointer, skPointer, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	sk = *skPointer
	pk = *pkPointer
	return
}

// SignHash signs a message using a secret key. An error is returned if the
// secret key is nil.
func SignHash(data Hash, sk SecretKey) (sig Signature, err error) {
	skNorm := [SecretKeySize]byte(sk)
	sig = *ed25519.Sign(&skNorm, data[:])
	return
}

// VerifyHash uses a public key and input data to verify a signature. And error
// is returned if the public key or signature is nil.
func VerifyHash(data Hash, pk PublicKey, sig Signature) (err error) {
	pkNorm := [PublicKeySize]byte(pk)
	sigNorm := [SignatureSize]byte(sig)
	verifies := ed25519.Verify(&pkNorm, data[:], &sigNorm)
	if !verifies {
		err = ErrInvalidSignature
		return
	}

	return
}
