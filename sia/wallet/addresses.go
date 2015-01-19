package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// TimelockedCoinAddress returns an address that can only be spent after block
// `unlockHeight`.
func (w *Wallet) timelockedCoinAddress(unlockHeight consensus.BlockHeight) (coinAddress consensus.CoinAddress, spendConditions consensus.SpendConditions, err error) {
	// Create the address + spend conditions.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return
	}
	spendConditions = consensus.SpendConditions{
		TimeLock:      unlockHeight,
		NumSignatures: 1,
		PublicKeys:    []crypto.PublicKey{pk},
	}
	coinAddress = spendConditions.CoinAddress()

	// Create a spendableAddress for the keys and add it to the
	// timelockedSpendableAddresses map. If the address has already been
	// unlocked, also add it to the list of currently spendable addresses. It
	// needs to go in both though in case there is a reorganization of the
	// blockchain.
	newSpendableAddress := &spendableAddress{
		spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
		spendConditions:  spendConditions,
		secretKey:        sk,
	}
	spendableAddressSlice := w.timelockedSpendableAddresses[unlockHeight]
	spendableAddressSlice = append(spendableAddressSlice, newSpendableAddress)
	w.timelockedSpendableAddresses[unlockHeight] = spendableAddressSlice
	if unlockHeight <= w.state.Height() {
		w.spendableAddresses[coinAddress] = newSpendableAddress
	}

	err = w.save()
	if err != nil {
		return
	}

	return
}

// coinAddress implements the core.Wallet interface.
func (w *Wallet) coinAddress() (coinAddress consensus.CoinAddress, spendConditions consensus.SpendConditions, err error) {
	// Create the keys and address.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return
	}
	spendConditions = consensus.SpendConditions{
		NumSignatures: 1,
		PublicKeys:    []crypto.PublicKey{pk},
	}
	coinAddress = spendConditions.CoinAddress()

	// Add the address to the set of spendable addresses.
	newSpendableAddress := &spendableAddress{
		spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
		spendConditions:  spendConditions,
		secretKey:        sk,
	}
	w.spendableAddresses[coinAddress] = newSpendableAddress

	err = w.save()
	if err != nil {
		return
	}

	return
}

// TimelockedCoinAddress returns an address that can only be spent after block
// `unlockHeight`.
func (w *Wallet) TimelockedCoinAddress(unlockHeight consensus.BlockHeight) (coinAddress consensus.CoinAddress, spendConditions consensus.SpendConditions, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timelockedCoinAddress(unlockHeight)
}

// CoinAddress implements the core.Wallet interface.
func (w *Wallet) CoinAddress() (coinAddress consensus.CoinAddress, spendConditions consensus.SpendConditions, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.coinAddress()
}
