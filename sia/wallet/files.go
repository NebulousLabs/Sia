package wallet

import (
	"io/ioutil"
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/signatures"
)

// AddressKey is how we serialize and store spendable addresses on
// disk.
type AddressKey struct {
	SpendConditions consensus.SpendConditions
	SecretKey       signatures.SecretKey
}

func (w *Wallet) save() (err error) {
	// Add every known spendable address + secret key.
	var i int
	keys := make([]AddressKey, len(w.spendableAddresses))
	for _, spendableAddress := range w.spendableAddresses {
		key := AddressKey{
			SpendConditions: spendableAddress.spendConditions,
			SecretKey:       spendableAddress.secretKey,
		}
		keys[i] = key
		i++
	}

	//  write the file
	fileData := encoding.Marshal(keys)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(w.saveFilename, fileData, 0666)
	if err != nil {
		return
	}

	return
}

// Save implements the core.Wallet interface.
func (w *Wallet) Save() (err error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.save()
}

// Load implements the core.Wallet interface.
func (w *Wallet) Load(filename string) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

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
	return
}
