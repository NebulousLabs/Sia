package sia

import (
	"crypto/rand"
	"errors"
	"math/big"
)

// the Host struct is kept in the client package because it's what the client
// uses to weigh hosts and pick them out when storing files.
type Host struct {
	IPAddress string
	Freeze    Currency
	Burn      Currency
	Price     Currency
	Duration  BlockHeight
	MinSize   uint64
	MaxSize   uint64
}

func (h *Host) Weight() Currency {
	return h.Freeze * h.Burn / h.Price
}

// Wallet.ClientFundFileContract() takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (w *Wallet) ClientFundFileContract(params *FileContractParameters, state *State) (err error) {
	// Scan the blockchain for outputs.
	w.Scan(state)

	// Add money to the transaction to fund the client's portion of the contract fund.
	err = w.FundTransaction(params.ClientContribution, &params.Transaction)
	if err != nil {
		return
	}

	return
}

// ChooseHost orders the hosts by weight and picks one at random.
func (w *Wallet) ChooseHost(state *State) (h Host, err error) {
	if len(state.HostList) == 0 {
		err = errors.New("no hosts found")
		return
	}
	if state.TotalWeight == 0 {
		panic("state has 0 total weight but not 0 length host list?")
	}

	// Get a random number between 0 and state.TotalWeight and then scroll
	// through state.HostList until at least that much weight has been passed.
	randInt, err := rand.Int(rand.Reader, big.NewInt(int64(state.TotalWeight)))
	if err != nil {
		return
	}
	randCurrency := Currency(randInt.Int64())
	weightPassed := Currency(0)
	var i int
	for i = 0; randCurrency >= weightPassed; i++ {
		weightPassed += state.HostList[i].Weight()
	}

	h = state.HostList[i]
	return
}
