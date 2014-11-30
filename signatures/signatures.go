package signatures

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"
)

type (
	SecretKey [32]byte
	PublicKey [64]byte
	Signature [64]byte
)

// GenerateKeyPair creates a public-secret keypair that can be used to sign and
// verify messages.
func GenerateKeyPair() (sk SecretKey, pk PublicKey, err error) {
	// Get the ecdsa keys.
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Copy the secret key into a byte array.
	skBytes := ecdsaKey.D.Bytes()
	copy(sk[:], skBytes)

	// Copy the public key into a byte array.
	pkBytes := ecdsaKey.PublicKey.X.Bytes()
	pkBytes = append(pkBytes, ecdsaKey.PublicKey.Y.Bytes()...)
	copy(pk[:], pkBytes)

	return
}

// SignBytes signs a message using a secret key.
func SignBytes(data []byte, sk SecretKey) (sig Signature, err error) {
	// Convert sk to an ecdsa.PrivateKey
	ecdsaKey := new(ecdsa.PrivateKey)
	ecdsaKey.PublicKey.Curve = elliptic.P256()
	ecdsaKey.D = new(big.Int).SetBytes(sk[:])

	// Get the signature.
	r, s, err := ecdsa.Sign(rand.Reader, ecdsaKey, data)
	if err != nil {
		return
	}

	// Convert the signature to a byte array.
	sigBytes := r.Bytes()
	sigBytes = append(sigBytes, s.Bytes()...)
	copy(sig[:], sigBytes)

	return
}

// VerifyBytes uses a public key and input data to verify a signature.
func VerifyBytes(data []byte, pk PublicKey, sig Signature) bool {
	// Get the public key.
	ecdsaKey := new(ecdsa.PublicKey)
	ecdsaKey.Curve = elliptic.P256()
	ecdsaKey.X = new(big.Int).SetBytes(pk[:32])
	ecdsaKey.Y = new(big.Int).SetBytes(pk[32:])
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(ecdsaKey, data, r, s)
}
