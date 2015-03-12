package wallet

import (
	"errors"
	"io/ioutil"
	"path/filepath"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

type savedKey struct {
	SecretKey        crypto.SecretKey
	UnlockConditions consensus.UnlockConditions
}

// save writes the contents of a wallet to a file.
func (w *Wallet) save() (err error) {
	// Convert the key map to a slice.
	keySlice := make([]savedKey, 0, len(w.keys))
	for _, key := range w.keys {
		keySlice = append(keySlice, savedKey{key.secretKey, key.unlockConditions})
	}
	walletData := encoding.Marshal(keySlice)

	// Write the wallet data to a backup file, in case something goes wrong
	err = ioutil.WriteFile(filepath.Join(w.saveDir, "wallet.backup"), walletData, 0666)
	if err != nil {
		return
	}
	// Overwrite the wallet file.
	err = ioutil.WriteFile(filepath.Join(w.saveDir, "wallet.dat"), walletData, 0666)
	if err != nil {
		// TODO: instruct user to recover wallet from the backup file
		return
	}

	return
}

// load reads the contents of a wallet from a file.
func (w *Wallet) load() (err error) {
	contents, err := ioutil.ReadFile(filepath.Join(w.saveDir, "wallet.dat"))
	if err != nil {
		return
	}
	var savedKeys []savedKey
	err = encoding.Unmarshal(contents, &savedKeys)
	if err != nil {
		return errors.New("corrupted wallet file")
	}

	height := w.state.Height()
	for _, skey := range savedKeys {
		// Create an entry in w.keys for each savedKey.
		w.keys[skey.UnlockConditions.UnlockHash()] = &key{
			spendable:        height >= skey.UnlockConditions.Timelock,
			unlockConditions: skey.UnlockConditions,
			secretKey:        skey.SecretKey,
			outputs:          make(map[consensus.SiacoinOutputID]*knownOutput),
		}

		// If Timelock != 0, also add to set of timelockedKeys.
		if tl := skey.UnlockConditions.Timelock; tl != 0 {
			w.timelockedKeys[tl] = append(w.timelockedKeys[tl], skey.UnlockConditions.UnlockHash())
		}
	}

	// To calculate the outputs for each key, we need to scan the entire
	// blockchain. This is done by setting w.recentBlock to the genesis block
	// and calling w.update.
	genesisBlock, exists := w.state.BlockAtHeight(0)
	if !exists {
		return errors.New("could not fetch genesis block")
	}
	w.recentBlock = genesisBlock.ID()
	w.update()
	return
}
