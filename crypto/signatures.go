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

// KeyPairGenerator allows the caller to generate public-secret key pairs.
type KeyPairGenerator interface {
	Generate() (SecretKey, PublicKey, error)
	GenerateDeterministic(entropy [EntropySize]byte) (SecretKey, PublicKey)
}

// entropySource allows the caller to retrieve an array of bytes populated to
// random values.
type entropySource interface {
	getEntropy() ([EntropySize]byte, error)
}

// keyDeriver allows the caller to generate a public-secret key pair based on
// provided entropy.
type keyDeriver interface {
	deriveKeyPair([EntropySize]byte) (SecretKey, PublicKey)
}

// stdGenerator is an implementation of KeyPairGenerator, allowing the caller
// to generate public-secret key pairs.
type stdGenerator struct {
	es entropySource
	kd keyDeriver
}

// Generate creates a public-secret keypair that can be used to sign and verify
// messages.
func (sg stdGenerator) Generate() (sk SecretKey, pk PublicKey, err error) {
	entropy, err := sg.es.getEntropy()
	if err != nil {
		return
	}
	sk, pk = sg.kd.deriveKeyPair(entropy)
	return sk, pk, nil
}

// GenerateDeterministic generates keys deterministically using the input
// entropy. The input entropy must be 32 bytes in length.
func (sg stdGenerator) GenerateDeterministic(entropy [EntropySize]byte) (SecretKey, PublicKey) {
	return sg.kd.deriveKeyPair(entropy)
}

// randSource is an implementation of entropySource that uses rand.Read to
// generate random bytes.
type randSource struct{}

// getEntropy returns an array of bytes with random values.
func (rs randSource) getEntropy() (entropy [EntropySize]byte, err error) {
	if _, err := rand.Read(entropy[:]); err != nil {
		return entropy, err
	}
	return entropy, nil
}

// ed25519Deriver is an implementation of keyDeriver that uses
// ed25519.GenerateKey to derive keys.
type ed25519Deriver struct{}

// deriveKeyPair derives a public-secret key pair derived from the provided
// array of bytes.
func (ed ed25519Deriver) deriveKeyPair(entropy [EntropySize]byte) (sk SecretKey, pk PublicKey) {
	skPointer, pkPointer := ed25519.GenerateKey(entropy)
	return *skPointer, *pkPointer
}

// StdKeyGen is a KeyPairGenerator based on randSource and ed25519Deriver.
var StdKeyGen KeyPairGenerator = stdGenerator{es: randSource{}, kd: ed25519Deriver{}}

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
