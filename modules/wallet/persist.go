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
}

// save writes the contents of a wallet to a file.
func (w *Wallet) save() error {
	// Convert the key map to a slice.
	keySlice := make([]savedKey, 0, len(w.keys))
	for _, key := range w.keys {
		keySlice = append(keySlice, savedKey{key.secretKey, key.unlockConditions})
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
	err := encoding.ReadFile(filepath.Join(w.saveDir, "wallet.dat"), &savedKeys)
	if err != nil {
		return err
	}

	height := w.state.Height()
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
	}

	// TODO TODO TODO: Need a scan or 're-scan' function.

	return nil
}
