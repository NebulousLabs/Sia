package sia

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
)

func GenerateKeyPair() (sk *ecdsa.PrivateKey, pk PublicKey, err error) {
	sk, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}
	pk = PublicKey(sk.PublicKey)
	return
}

func SignBytes(data []byte, sk *ecdsa.PrivateKey) (sig Signature, err error) {
	sig.R, sig.S, err = ecdsa.Sign(rand.Reader, sk, data)
	return
}

func VerifyBytes(data []byte, pubKey PublicKey, sig Signature) bool {
	return ecdsa.Verify((*ecdsa.PublicKey)(&pubKey), data, sig.R, sig.S)
}
