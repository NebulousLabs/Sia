package wallet

import (
	// "io/ioutil"
	// "os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	// "github.com/NebulousLabs/Sia/encoding"
)

// Key is how we serialize and store spendable addresses on disk.
type Key struct {
	SpendConditions consensus.SpendConditions
	SecretKey       crypto.SecretKey
}

func (w *Wallet) save() (err error) {
	/*
		// Add every known spendable address + secret key.
		var i int
		keys := make([]Key, len(w.addresses))
		for _, address := range w.addresses {
			key := Key{
				SpendConditions: address.spendConditions,
				SecretKey:       address.secretKey,
			}
			keys[i] = key
			i++
		}

		timelockedKeys := make([]AddressKey, len(w.timelockedAddresses))
		for _, address := range w.timelockedAddresses {
			key := Key{
				SpendConditions: address.spendConditions,
				SecretKey:       address.secretKey,
			}
			timelockedKeys[i] = key
			i++
		}

		// Write the file.
		fileData := encoding.MarshalAll(keys, timelockedKeys)
		if err != nil {
			return
		}
		// TODO: Instead of using WriteFile, write to a temp file and then do a move,
		// this is an action that should probably appear in a library somewhere.
		err = ioutil.WriteFile(w.saveFilename, fileData, 0666)
		if err != nil {
			return
		}
	*/

	return
}

// Save implements the core.Wallet interface.
func (w *Wallet) Save() (err error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.save()
}

// Load implements the core.Wallet interface.
//
// TODO TODO TODO: update load so that it also loads timelocked keys.
func (w *Wallet) Load(filename string) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	/*
		// Check if the file exists, then read it into memory.
		//
		// If there is no existing wallet file, the wallet assumes that the higher
		// level process (core or daemon) has already approved the filename, and
		// that a wallet simply doesn't exist yet. A new wallet file will be
		// created next time wallet.Save() is called.
		//
		// TODO: wallet should not return nil upon load if the file it's trying to
		// load doesn't exist.
		if _, err = os.Stat(filename); os.IsNotExist(err) {
			err = nil
			return
		}
		contents, err := ioutil.ReadFile(filename)
		if err != nil {
			return
		}

		// Unmarshal the spendable addresses and put them into the wallet.
		var keys []AddressKey
		err = encoding.Unmarshal(contents, &keys)
		if err != nil {
			return
		}
		for _, key := range keys {
			newSpendableAddress := &spendableAddress{
				spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
				spendConditions:  key.SpendConditions,
				secretKey:        key.SecretKey,
			}
			w.spendableAddresses[key.SpendConditions.CoinAddress()] = newSpendableAddress
		}
	*/
	return
}
