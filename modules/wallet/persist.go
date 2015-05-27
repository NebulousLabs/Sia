package wallet

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

type savedKey struct {
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
	Visible          bool
}

// legacySavedKey preserves compatibility with the other beta wallets. After
// the full currency is launched, this struct and the related code can be
// discarded.
type legacySavedKey struct {
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
}

// save writes the contents of a wallet to a file.
func (w *Wallet) save() error {
	// Convert the key map to a slice.
	keySlice := make([]savedKey, 0, len(w.keys))
	for _, key := range w.keys {
		_, exists := w.visibleAddresses[key.unlockConditions.UnlockHash()]
		keySlice = append(keySlice, savedKey{key.secretKey, key.unlockConditions, exists})
	}

	// Write the wallet data to a backup file, in case something goes wrong
	err := encoding.WriteFile(filepath.Join(w.saveDir, "wallet.backup"), keySlice)
	if err != nil {
		return err
	}
	// Overwrite the wallet file.
	err = encoding.WriteFile(filepath.Join(w.saveDir, "wallet.dat"), keySlice)
	if err != nil {
		// TODO: instruct user to recover wallet from the backup file
		return err
	}

	return nil
}

// load reads the contents of a wallet from a file.
func (w *Wallet) load() error {
	var savedKeys []savedKey
	var legacyKeys []legacySavedKey
	err := encoding.ReadFile(filepath.Join(w.saveDir, "wallet.dat"), &savedKeys)
	if err != nil {
		err = encoding.ReadFile(filepath.Join(w.saveDir, "wallet.dat"), &legacyKeys)
		if err != nil {
			return err
		}

		for _, key := range legacyKeys {
			savedKeys = append(savedKeys, savedKey{key.SecretKey, key.UnlockConditions, false})
		}
	}

	height := w.consensusHeight
	for _, skey := range savedKeys {
		// Create an entry in w.keys for each savedKey.
		w.keys[skey.UnlockConditions.UnlockHash()] = &key{
			spendable:        height >= skey.UnlockConditions.Timelock,
			unlockConditions: skey.UnlockConditions,
			secretKey:        skey.SecretKey,
			outputs:          make(map[types.SiacoinOutputID]*knownOutput),
		}

		// If Timelock != 0, also add to set of timelockedKeys.
		if tl := skey.UnlockConditions.Timelock; tl != 0 {
			w.timelockedKeys[tl] = append(w.timelockedKeys[tl], skey.UnlockConditions.UnlockHash())
		}

		if skey.Visible {
			w.visibleAddresses[skey.UnlockConditions.UnlockHash()] = struct{}{}
		}
	}

	// If there are no visible addresses, create one.
	if len(w.visibleAddresses) == 0 {
		_, _, err := w.coinAddress(true)
		if err != nil {
			return err
		}
	}

	return nil
}
