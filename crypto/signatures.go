package crypto

import (
	"errors"
	"io"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/ed25519"
	"github.com/NebulousLabs/fastrand"
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

var (
	// ErrInvalidSignature is returned if a signature is provided that does not
	// match the data and public key.
	ErrInvalidSignature = errors.New("invalid signature")
)

type (
	// PublicKey is an object that can be used to verify signatures.
	PublicKey [PublicKeySize]byte

	// SecretKey can be used to sign data for the corresponding public key.
	SecretKey [SecretKeySize]byte

	// Signature proves that data was signed by the owner of a particular
	// public key's corresponding secret key.
	Signature [SignatureSize]byte
)

// PublicKey returns the public key that corresponds to a secret key.
func (sk SecretKey) PublicKey() (pk PublicKey) {
	copy(pk[:], sk[SecretKeySize-PublicKeySize:])
	return
}

// GenerateKeyPair creates a public-secret keypair that can be used to sign and verify
// messages.
func GenerateKeyPair() (sk SecretKey, pk PublicKey) {
	var entropy [EntropySize]byte
	fastrand.Read(entropy[:])
	return GenerateKeyPairDeterministic(entropy)
}

// GenerateKeyPairDeterministic generates keys deterministically using the input
// entropy. The input entropy must be 32 bytes in length.
func GenerateKeyPairDeterministic(entropy [EntropySize]byte) (SecretKey, PublicKey) {
	sk, pk := ed25519.GenerateKey(entropy)
	return *sk, *pk
}

// ReadSignedObject reads a length-prefixed object prefixed by its signature,
// and verifies the signature.
func ReadSignedObject(r io.Reader, obj interface{}, maxLen uint64, pk PublicKey) error {
	// read the signature
	var sig Signature
	err := encoding.NewDecoder(r).Decode(&sig)
	if err != nil {
		return err
	}
	// read the encoded object
	encObj, err := encoding.ReadPrefix(r, maxLen)
	if err != nil {
		return err
	}
	// verify the signature
	if err := VerifyHash(HashBytes(encObj), pk, sig); err != nil {
		return err
	}
	// decode the object
	return encoding.Unmarshal(encObj, obj)
}

// SignHash signs a message using a secret key.
func SignHash(data Hash, sk SecretKey) Signature {
	skNorm := [SecretKeySize]byte(sk)
	return *ed25519.Sign(&skNorm, data[:])
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

// WriteSignedObject writes a length-prefixed object prefixed by its signature.
func WriteSignedObject(w io.Writer, obj interface{}, sk SecretKey) error {
	objBytes := encoding.Marshal(obj)
	sig := SignHash(HashBytes(objBytes), sk)
	return encoding.NewEncoder(w).EncodeAll(sig, objBytes)
}
