package crypto

import (
	"crypto/rand"
	"errors"
	"io"

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

type (
	// keyDeriver allows the caller to generate a public-secret key pair based on
	// provided entropy.
	keyDeriver interface {
		deriveKeyPair([EntropySize]byte) (ed25519.SecretKey, ed25519.PublicKey)
	}

	// stdGenerator is an implementation of KeyPairGenerator, allowing the caller
	// to generate public-secret key pairs.
	stdGenerator struct {
		entropySource io.Reader
		kd            keyDeriver
	}
)

// Generate creates a public-secret keypair that can be used to sign and verify
// messages.
func (sg stdGenerator) Generate() (sk SecretKey, pk PublicKey, err error) {
	var entropy [EntropySize]byte
	_, err = sg.entropySource.Read(entropy[:])
	if err != nil {
		return
	}

	skPointer, pkPointer := sg.kd.deriveKeyPair(entropy)
	return *skPointer, *pkPointer, nil
}

// GenerateDeterministic generates keys deterministically using the input
// entropy. The input entropy must be 32 bytes in length.
func (sg stdGenerator) GenerateDeterministic(entropy [EntropySize]byte) (SecretKey, PublicKey) {
	skPointer, pkPointer := sg.kd.deriveKeyPair(entropy)
	return *skPointer, *pkPointer
}

// ed25519Deriver is an implementation of keyDeriver that uses
// ed25519.GenerateKey to derive keys.
type ed25519Deriver struct{}

// deriveKeyPair derives a public-secret key pair derived from the provided
// array of bytes.
func (ed ed25519Deriver) deriveKeyPair(entropy [EntropySize]byte) (ed25519.SecretKey, ed25519.PublicKey) {
	return ed25519.GenerateKey(entropy)
}

// StdKeyGen is a stdGenerator based on randSource and ed25519Deriver.
var StdKeyGen stdGenerator = stdGenerator{entropySource: rand.Reader, kd: ed25519Deriver{}}

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
