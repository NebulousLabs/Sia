package wallet

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// a savedKey contains a single-signature key and all the tools needed to spend
// outputs at its address.
type savedKey struct {
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
	Visible          bool
}

// saveKeys saves the current set of keys known to the wallet to a file.
func (w *Wallet) saveKeys(filepath string) error {
	// Convert the key map to a slice and write to disk.
	keySlice := make([]savedKey, 0, len(w.keys))
	for _, key := range w.keys {
		_, exists := w.visibleAddresses[key.unlockConditions.UnlockHash()]
		keySlice = append(keySlice, savedKey{key.secretKey, key.unlockConditions, exists})
	}
	return encoding.WriteFile(filepath, keySlice)
}

// saveSiafundTracking save the addresses that track siafunds.
func (w *Wallet) saveSiafundTracking(filepath string) error {
	// Put the siafund tracking addresses into a slice and write to disk.
	siafundSlice := make([]types.UnlockHash, 0, len(w.siafundAddresses))
	for sa, _ := range w.siafundAddresses {
		siafundSlice = append(siafundSlice, sa)
	}
	// outputs.dat is intentionally a bit of a misleading name. If I called it
	// 'siafunds.dat' or something similar, people might think it's okay to
	// delete their siafund keys, which is NOT okay. Instead of potentially
	// having this confusion, I chose a less suggestive name.
	return encoding.WriteFile(filepath, siafundSlice)
}

// save writes the contents of a wallet to a file.
func (w *Wallet) save() error {
	// Save the siacoin keys to disk.
	err := w.saveKeys(filepath.Join(w.saveDir, "wallet.backup"))
	if err != nil {
		return err
	}
	err = w.saveKeys(filepath.Join(w.saveDir, "wallet.dat"))
	if err != nil {
		return err
	}

	// Create a second file for the siafunds. This is another
	// deprecated-on-arrival file.
	err = w.saveSiafundTracking(filepath.Join(w.saveDir, "outputs.backup"))
	if err != nil {
		return err
	}
	// Save the primary copy.
	err = w.saveSiafundTracking(filepath.Join(w.saveDir, "outputs.dat"))
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

// loadWallet pulls a wallet from disk into memory, merging it with whatever
// wallet is already in memory. The result is a combined wallet that has all of
// the addresses.
func (w *Wallet) loadWallet(filepath string) error {
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

// loadSiafundTracking loads siafund addresses for tracking.
func (w *Wallet) loadSiafundTracking(filepath string) error {
	// Load the siafunds file, which is intentionally called 'outputs.dat'.
	var siafundAddresses []types.UnlockHash
	err := encoding.ReadFile(filepath, &siafundAddresses)
	if err != nil {
		return err
	}

	// Load the addresses into the wallet.
	for _, sa := range siafundAddresses {
		w.siafundAddresses[sa] = struct{}{}
	}
	return nil
}

// load reads the contents of a wallet from a file.
func (w *Wallet) load() error {
	err := w.loadWallet(filepath.Join(w.saveDir, "wallet.dat"))
	if err != nil {
		// try loading the backup
		// TODO: display/log a warning
		err = w.loadWallet(filepath.Join(w.saveDir, "wallet.backup"))
		if err != nil {
			return err
		}
	}

	// Load the siafunds file, which is intentionally called 'outputs.dat'.
	err = w.loadSiafundTracking(filepath.Join(w.saveDir, "outputs.dat"))
	if err != nil {
		// try loading the backup
		// TODO: display/log a warning?
		err = w.loadSiafundTracking(filepath.Join(w.saveDir, "outputs.backup"))
		if err != nil {
			return err
		}
	}

	return nil
}

// MergeWallet merges another wallet with the already-loaded wallet, creating a
// new wallet that contains all of the addresses from each. This is useful for
// loading backups.
func (w *Wallet) MergeWallet(filepath string) error {
	return w.loadWallet(filepath)
}
