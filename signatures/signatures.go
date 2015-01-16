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
	PublicKey [32]byte
	SecretKey [64]byte
	Signature [64]byte
)

// GenerateKeyPair creates a public-secret keypair that can be used to sign and
// verify messages.
func GenerateKeyPair() (sk SecretKey, pk PublicKey, err error) {
	edPK, edSK, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	copy(sk[:], edSK[:])
	copy(pk[:], edPK[:])
	return
}

// SignBytes signs a message using a secret key.
func SignBytes(data []byte, sk SecretKey) (sig Signature, err error) {
	edSK := [64]byte(sk)
	edSig := ed25519.Sign(&edSK, data)
	copy(sig[:], edSig[:])
	return
}

// VerifyBytes uses a public key and input data to verify a signature.
func VerifyBytes(data []byte, pk PublicKey, sig Signature) bool {
	edPK := [32]byte(pk)
	edSig := [64]byte(sig)
	return ed25519.Verify(&edPK, data, &edSig)
}
