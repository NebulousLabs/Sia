package sia

// wallet.go contains things like signatures and scans the blockchain for
// available funds that can be spent.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
)

type Wallet struct {
	SecretKey *ecdsa.PrivateKey

	SpendConditions SpendConditions

	// OwnedOutputs is a list of outputs that can spent using the secret
	// key.
	// OwnedOutputs map[OutputID]Output
}

func CreateWallet() (w *Wallet, err error) {
	w = new(Wallet)

	curve := elliptic.P256()
	w.SecretKey, err = ecdsa.GenerateKey(curve, rand.Reader)

	w.SpendConditions.NumSignatures = 1
	w.SpendConditions.PublicKeys = make([]PublicKey, 1)
	w.SpendConditions.PublicKeys[0] = PublicKey(w.SecretKey.PublicKey)

	return
}

func (w *Wallet) GetAddress() (ca CoinAddress) {
	// ca = HashStruct(w.SpendConditions)
	return
}
