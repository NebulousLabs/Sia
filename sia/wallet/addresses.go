package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/signatures"
)

// timelockedCoinAddress returns a CoinAddress with a timelock, as well as the
// conditions needed to spend it.
func (w *BasicWallet) timelockedCoinAddress(release consensus.BlockHeight) (spendConditions consensus.SpendConditions, err error) {
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

// CoinAddress implements the core.BasicWallet interface.
func (w *BasicWallet) CoinAddress() (coinAddress consensus.CoinAddress, err error) {
	w.Lock()
	defer w.Unlock()

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
	err = w.Save()
	return
}
