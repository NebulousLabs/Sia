package wallet

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

type savedKey struct {
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
	Visible          bool
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
	// Save the primary copy.
	err = encoding.WriteFile(filepath.Join(w.saveDir, "wallet.dat"), keySlice)
	if err != nil {
		// TODO: instruct user to recover wallet from the backup file
		return err
	}

	// Create a second file for the siafunds. This is another
	// deprecated-on-arrival file.
	siafundSlice := make([]types.UnlockHash, 0, len(w.siafundAddresses))
	for sa, _ := range w.siafundAddresses {
		siafundSlice = append(siafundSlice, sa)
	}
	// outputs.dat is intentionally a bit of a misleading name. If I called it
	// 'siafunds.dat' or something similar, people might think it's okay to
	// delete their siafund keys, which is NOT okay. Instead of potentially
	// having this confusion, I chose a less suggestive name.
	err = encoding.WriteFile(filepath.Join(w.saveDir, "outputs.backup"), siafundSlice)
	if err != nil {
		return err
	}
	// Save the primary copy.
	err = encoding.WriteFile(filepath.Join(w.saveDir, "outputs.dat"), siafundSlice)
	if err != nil {
		return err
	}

	return nil
}

// loadKeys takes a set of keys and loads them into the wallet.
func (w *Wallet) loadKeys(savedKeys []savedKey) error {
	height := w.consensusHeight
	for _, skey := range savedKeys {
		// Skip this key if it's already known to the wallet.
		_, exists := w.keys[skey.UnlockConditions.UnlockHash()]
		if exists {
			continue
		}

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

	w.save()

	return nil
}

// load reads the contents of a wallet from a file.
func (w *Wallet) load() error {
	var savedKeys []savedKey
	err := encoding.ReadFile(filepath.Join(w.saveDir, "wallet.dat"), &savedKeys)
	// never load from the home folder during testing
	if err != nil {
		// try loading the backup
		// TODO: display/log a warning
		err = encoding.ReadFile(filepath.Join(w.saveDir, "wallet.backup"), &savedKeys)
		if err != nil {
			return err
		}
	}
	err = w.loadKeys(savedKeys)
	if err != nil {
		return err
	}

	// Load the siafunds file, which is intentionally called 'outputs.dat'.
	var siafundAddresses []types.UnlockHash
	err = encoding.ReadFile(filepath.Join(w.saveDir, "outputs.backup"), &siafundAddresses)
	// never load from the home folder during testing
	if build.Release != "release" || err != nil {
		// try loading the backup
		// TODO: display/log a warning?
		err = encoding.ReadFile(filepath.Join(w.saveDir, "outputs.dat"), &siafundAddresses)
		if err != nil {
			return err
		}
	}
	for _, sa := range siafundAddresses {
		w.siafundAddresses[sa] = struct{}{}
	}

	return nil
}

// MergeWallet merges another wallet with the already-loaded wallet, creating a
// new wallet that contains all of the addresses from each. This is useful for
// loading backups.
func (w *Wallet) MergeWallet(filepath string) error {
	var savedKeys []savedKey
	err := encoding.ReadFile(filepath, &savedKeys)
	if err != nil {
		return err
	}
	err = w.loadKeys(savedKeys)
	if err != nil {
		return err
	}
	return nil
}
