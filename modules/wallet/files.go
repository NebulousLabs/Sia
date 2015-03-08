package wallet

import (
	"errors"
	"io/ioutil"
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

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

	// Write the data to a temp file
	err = ioutil.WriteFile(w.filename+"_temp", encoding.Marshal(keySlice), 0666)
	if err != nil {
		return
	}
	// Atomically overwrite the old wallet file with the new wallet file.
	err = os.Rename(w.filename+"_temp", w.filename)
	if err != nil {
		// TODO: instruct user to recover wallet from w.filename+"_temp"
		return
	}

	return
}

// load reads the contents of a wallet from a file.
func (w *Wallet) load(filename string) (err error) {
	contents, err := ioutil.ReadFile(filename)
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
