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

// Save implements the core.Wallet interface.
func (w *Wallet) Save() (err error) {
	w.rLock()
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
	w.rUnlock()

	//  write the file
	fileData := encoding.Marshal(keys)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(w.saveFilename, fileData, 0666)
	return
}

// Load implements the core.Wallet interface.
func (w *Wallet) Load(filename string) (err error) {
	// Check if the file exists, then read it into memory.
	if _, err = os.Stat(filename); os.IsNotExist(err) {
		err = nil
		return
	}
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	// Unmarshal the spendable addresses and put them into the wallet.
	w.lock()
	defer w.unlock()
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
