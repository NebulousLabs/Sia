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

	// OwnedOutputs is a list of outputs that can spent using the secret
	// key.
	OwnedOutputs map[OutputID]Output
}

func CreateWallet() (w *Wallet, err error) {
	w = new(Wallet)

	curve := elliptic.P256()
	w.SecretKey, err = ecdsa.GenerateKey(curve, rand.Reader)
	return
}

func (w *Wallet) GetAddress() (ca CoinAddress) {
	var sc SpendConditions
	sc.NumSignatures = 1
	sc.PublicKeys = make([]PublicKey, 1)
	sc.PublicKeys[0] = PublicKey(w.SecretKey.PublicKey)

	// ca = HashStruct(SpendConditions)
	// SpendConditions should be a merkle tree.

	return
}
