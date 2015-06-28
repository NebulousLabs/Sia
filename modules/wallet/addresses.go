package wallet

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// TimelockedCoinAddress returns an address that can only be spent after block
// `unlockHeight`.
func (w *Wallet) timelockedCoinAddress(unlockHeight types.BlockHeight, visible bool) (coinAddress types.UnlockHash, unlockConditions types.UnlockConditions, err error) {
	// Create the address + spend conditions.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return
	}
	unlockConditions = types.UnlockConditions{
		Timelock:           unlockHeight,
		SignaturesRequired: 1,
		PublicKeys: []types.SiaPublicKey{
			types.SiaPublicKey{
				Algorithm: types.SignatureEd25519,
				Key:       encoding.Marshal(pk),
			},
		},
	}
	coinAddress = unlockConditions.UnlockHash()

	// Create a spendableAddress for the keys and add it to the
	// timelockedUnlockableAddresses map. If the address has already been
	// unlocked, also add it to the list of currently spendable addresses. It
	// needs to go in both though in case there is a reorganization of the
	// blockchain.
	w.keys[coinAddress] = &key{
		spendable:        w.consensusHeight >= unlockHeight,
		unlockConditions: unlockConditions,
		secretKey:        sk,

		outputs: make(map[types.SiacoinOutputID]*knownOutput),
	}

	// Add this key to the list of addresses that get unlocked at
	// `unlockHeight`
	w.timelockedKeys[unlockHeight] = append(w.timelockedKeys[unlockHeight], coinAddress)

	// Put the address in the list of visible addresses if 'visible' is set.
	if visible {
		w.visibleAddresses[coinAddress] = struct{}{}
	}

	// Save the wallet state, which now includes the new address.
	err = w.save()
	if err != nil {
		return
	}

	return
}

// coinAddress returns a new address for receiving coins.
func (w *Wallet) coinAddress(visible bool) (coinAddress types.UnlockHash, unlockConditions types.UnlockConditions, err error) {
	// Create the keys and address.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return
	}
	unlockConditions = types.UnlockConditions{
		SignaturesRequired: 1,
		PublicKeys: []types.SiaPublicKey{
			types.SiaPublicKey{
				Algorithm: types.SignatureEd25519,
				Key:       encoding.Marshal(pk),
			},
		},
	}
	coinAddress = unlockConditions.UnlockHash()

	// Add the address to the set of spendable addresses.
	newKey := &key{
		spendable:        true,
		unlockConditions: unlockConditions,
		secretKey:        sk,

		outputs: make(map[types.SiacoinOutputID]*knownOutput),
	}
	w.keys[coinAddress] = newKey

	if visible {
		w.visibleAddresses[coinAddress] = struct{}{}
	}

	// Save the wallet state, which now includes the new address.
	err = w.save()
	if err != nil {
		return
	}

	return
}

// TimelockedCoinAddress returns an address that can only be spent after block
// `unlockHeight`.
func (w *Wallet) TimelockedCoinAddress(unlockHeight types.BlockHeight, visible bool) (coinAddress types.UnlockHash, unlockConditions types.UnlockConditions, err error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.timelockedCoinAddress(unlockHeight, visible)
}

// CoinAddress implements the core.Wallet interface.
func (w *Wallet) CoinAddress(visible bool) (coinAddress types.UnlockHash, unlockConditions types.UnlockConditions, err error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.coinAddress(visible)
}
