package signatures

import (
	"crypto/rand"

	"github.com/agl/ed25519"
)

// One thing that worries me about this file is that the library returns a
// bunch of pointers to data, but the types copy the data into other memory and
// pass that around instead. This may result in side channel attacks becoming
// possible.

type (
	PublicKey *[ed25519.PublicKeySize]byte
	SecretKey *[ed25519.PrivateKeySize]byte
	Signature *[ed25519.SignatureSize]byte
)

// GenerateKeyPair creates a public-secret keypair that can be used to sign and
// verify messages.
func GenerateKeyPair() (sk SecretKey, pk PublicKey, err error) {
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
