package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/signatures"
)

// TimelockedCoinAddress returns an address that can only be spent after block
// `unlockHeight`.
func (w *Wallet) TimelockedCoinAddress(unlockHeight consensus.BlockHeight) (coinAddress consensus.CoinAddress, spendConditions consensus.SpendConditions, err error) {
	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	spendConditions = consensus.SpendConditions{
		TimeLock:      unlockHeight,
		NumSignatures: 1,
		PublicKeys:    []signatures.PublicKey{pk},
	}
	coinAddress = spendConditions.CoinAddress()

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

// CoinAddress implements the core.Wallet interface.
func (w *Wallet) CoinAddress() (coinAddress consensus.CoinAddress, spendConditions consensus.SpendConditions, err error) {
	w.lock()
	defer w.unlock()

	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	spendConditions = consensus.SpendConditions{
		NumSignatures: 1,
		PublicKeys:    []signatures.PublicKey{pk},
	}
	coinAddress = spendConditions.CoinAddress()

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
