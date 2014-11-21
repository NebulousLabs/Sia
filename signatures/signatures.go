package signatures

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"

	"github.com/NebulousLabs/Andromeda/encoding"
)

type (
	PublicKey ecdsa.PublicKey
	SecretKey *ecdsa.PrivateKey
)

type Signature struct {
	R, S *big.Int
}

// GenerateKeyPair creates a public-secret keypair that can be used to sign and
// verify messages.
func GenerateKeyPair() (sk SecretKey, pk PublicKey, err error) {
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	sk = SecretKey(ecdsaKey)
	if err != nil {
		return
	}
	pk = PublicKey(sk.PublicKey)
	return
}

// SignBytes signs a message using a secret key.
func SignBytes(data []byte, sk SecretKey) (sig Signature, err error) {
	sig.R, sig.S, err = ecdsa.Sign(rand.Reader, sk, data)
	return
}

// VerifyBytes uses a public key and input data to verify a signature.
func VerifyBytes(data []byte, pubKey PublicKey, sig Signature) bool {
	return ecdsa.Verify((*ecdsa.PublicKey)(&pubKey), data, sig.R, sig.S)
}

// Signature.MarshalSia implements the Marshaler interface for Signatures.
func (s *Signature) MarshalSia() []byte {
	if s.R == nil || s.S == nil {
		return []byte{0, 0}
	}
	// pretend Signature is a tuple of []bytes
	// this lets us use Marshal instead of doing manual length-prefixing
	return encoding.Marshal(struct{ R, S []byte }{s.R.Bytes(), s.S.Bytes()})
}

// Signature.UnmarshalSia implements the Unmarshaler interface for Signatures.
func (s *Signature) UnmarshalSia(b []byte) int {
	// inverse of the struct trick used in Signature.MarshalSia
	str := struct{ R, S []byte }{}
	if encoding.Unmarshal(b, &str) != nil {
		return 0
	}
	s.R = new(big.Int).SetBytes(str.R)
	s.S = new(big.Int).SetBytes(str.S)
	return len(str.R) + len(str.S) + 2
}

// PublicKey.MarshalSia implements the Marshaler interface for PublicKeys.
func (pk PublicKey) MarshalSia() []byte {
	if pk.X == nil || pk.Y == nil {
		return []byte{0, 0}
	}
	// see Signature.MarshalSia
	return encoding.Marshal(struct{ X, Y []byte }{pk.X.Bytes(), pk.Y.Bytes()})
}

// PublicKey.UnmarshalSia implements the Unmarshaler interface for PublicKeys.
func (pk *PublicKey) UnmarshalSia(b []byte) int {
	// see Signature.UnmarshalSia
	str := struct{ X, Y []byte }{}
	if encoding.Unmarshal(b, &str) != nil {
		return 0
	}
	pk.X = new(big.Int).SetBytes(str.X)
	pk.Y = new(big.Int).SetBytes(str.Y)
	return len(str.X) + len(str.Y) + 2
}
