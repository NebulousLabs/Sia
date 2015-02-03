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
	PublicKey *[ed25519.PublicKeySize]byte
	SecretKey *[ed25519.PrivateKeySize]byte
	Signature *[ed25519.SignatureSize]byte
)

// DecodePublicKey takes a bytes slice and returns the corresponding public
// key. An error is returned if the byte slice is the wrong length.
func DecodePublicKey(encodedPK []byte) (pk PublicKey, err error) {
	if len(encodedPK) != PublicKeySize {
		err = errors.New("public key is invalid size")
		return
	}

	var epk [PublicKeySize]byte
	copy(epk[:], encodedPK)
	pk = PublicKey(&epk)
	return
}

func DecodeSignature(encodedSig []byte) (sig Signature, err error) {
	if len(encodedSig) != SignatureSize {
		err = errors.New("Signature is invalid size")
		return
	}

	var esig [SignatureSize]byte
	copy(esig[:], encodedSig)
	sig = Signature(&esig)
	return
}

// GenerateKeyPair creates a public-secret keypair that can be used to sign and
// verify messages.
func GenerateSignatureKeys() (sk SecretKey, pk PublicKey, err error) {
	pk, sk, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	return
}

// SignBytes signs a message using a secret key.
func SignBytes(data []byte, sk SecretKey) (sig Signature, err error) {
	sig = ed25519.Sign(sk, data)
	return
}

// VerifyBytes uses a public key and input data to verify a signature.
func VerifyBytes(data []byte, pk PublicKey, sig Signature) bool {
	return ed25519.Verify(pk, data, sig)
}
