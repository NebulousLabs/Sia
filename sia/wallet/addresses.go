package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/signatures"
)

// timelockedCoinAddress returns a CoinAddress with a timelock, as well as the
// conditions needed to spend it.
func (w *Wallet) timelockedCoinAddress(release consensus.BlockHeight) (spendConditions consensus.SpendConditions, err error) {
	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	spendConditions = consensus.SpendConditions{
		TimeLock:      release,
		NumSignatures: 1,
		PublicKeys:    []signatures.PublicKey{pk},
	}

	newSpendableAddress := &spendableAddress{
		spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
		spendConditions:  spendConditions,
		secretKey:        sk,
	}

	coinAddress := spendConditions.CoinAddress()
	w.spendableAddresses[coinAddress] = newSpendableAddress
	return
}

// CoinAddress implements the core.Wallet interface.
func (w *Wallet) CoinAddress() (coinAddress consensus.CoinAddress, err error) {
	w.lock()

	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	newSpendableAddress := &spendableAddress{
		spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
		spendConditions: consensus.SpendConditions{
			NumSignatures: 1,
			PublicKeys:    []signatures.PublicKey{pk},
		},
		secretKey: sk,
	}

	coinAddress = newSpendableAddress.spendConditions.CoinAddress()
	w.spendableAddresses[coinAddress] = newSpendableAddress

	w.unlock() // Unlock before saving.
	err = w.Save()
	if err != nil {
		return
	}

	return
}
