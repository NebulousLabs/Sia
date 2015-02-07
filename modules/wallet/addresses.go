package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

// TimelockedCoinAddress returns an address that can only be spent after block
// `unlockHeight`.
func (w *Wallet) timelockedCoinAddress(unlockHeight consensus.BlockHeight) (coinAddress consensus.UnlockHash, spendConditions consensus.UnlockConditions, err error) {
	// Create the address + spend conditions.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return
	}
	spendConditions = consensus.UnlockConditions{
		Timelock:      unlockHeight,
		NumSignatures: 1,
		PublicKeys: []consensus.SiaPublicKey{
			consensus.SiaPublicKey{
				Algorithm: consensus.SignatureEd25519,
				Key:       encoding.Marshal(pk),
			},
		},
	}
	coinAddress = spendConditions.UnlockHash()

	// Create a spendableAddress for the keys and add it to the
	// timelockedUnlockableAddresses map. If the address has already been
	// unlocked, also add it to the list of currently spendable addresses. It
	// needs to go in both though in case there is a reorganization of the
	// blockchain.
	newKey := &key{
		spendConditions: spendConditions,
		secretKey:       sk,

		outputs: make(map[consensus.SiacoinOutputID]*knownOutput),
	}
	if unlockHeight <= w.state.Height() {
		newKey.spendable = true
	}
	w.keys[coinAddress] = newKey

	// Add this key to the list of addresses that get unlocked at
	// `unlockHeight`
	heightAddrs := w.timelockedKeys[unlockHeight]
	heightAddrs = append(heightAddrs, coinAddress)
	w.timelockedKeys[unlockHeight] = heightAddrs

	// Save the wallet state, which now includes the new address.
	err = w.save()
	if err != nil {
		return
	}

	return
}

// coinAddress returns a new address for receiving coins.
func (w *Wallet) coinAddress() (coinAddress consensus.UnlockHash, spendConditions consensus.UnlockConditions, err error) {
	// Create the keys and address.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return
	}
	spendConditions = consensus.UnlockConditions{
		NumSignatures: 1,
		PublicKeys: []consensus.SiaPublicKey{
			consensus.SiaPublicKey{
				Algorithm: consensus.SignatureEd25519,
				Key:       encoding.Marshal(pk),
			},
		},
	}
	coinAddress = spendConditions.UnlockHash()

	// Add the address to the set of spendable addresses.
	newKey := &key{
		spendable:       true,
		spendConditions: spendConditions,
		secretKey:       sk,

		outputs: make(map[consensus.SiacoinOutputID]*knownOutput),
	}
	w.keys[coinAddress] = newKey

	// Save the wallet state, which now includes the new address.
	err = w.save()
	if err != nil {
		return
	}

	return
}

// TimelockedCoinAddress returns an address that can only be spent after block
// `unlockHeight`.
func (w *Wallet) TimelockedCoinAddress(unlockHeight consensus.BlockHeight) (coinAddress consensus.UnlockHash, spendConditions consensus.UnlockConditions, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timelockedCoinAddress(unlockHeight)
}

// CoinAddress implements the core.Wallet interface.
func (w *Wallet) CoinAddress() (coinAddress consensus.UnlockHash, spendConditions consensus.UnlockConditions, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.coinAddress()
}
