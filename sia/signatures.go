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
