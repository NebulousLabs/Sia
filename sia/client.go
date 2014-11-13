package sia

import (
	"crypto/rand"
	"errors"
	"math/big"
	"os"
)

// the Host struct is kept in the client package because it's what the client
// uses to weigh hosts and pick them out when storing files.
type Host struct {
	IPAddress   string
	MinSize     uint64
	MaxSize     uint64
	Duration    BlockHeight
	Frequency   BlockHeight
	Tolerance   uint64
	Price       Currency
	Burn        Currency
	Freeze      Currency
	CoinAddress CoinAddress
}

func (h *Host) Weight() Currency {
	return h.Freeze * h.Burn / h.Price
}

// Wallet.ClientFundFileContract() takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (w *Wallet) ClientProposeContract(filename string, state *State) (err error) {
	// Scan the blockchain for outputs.
	w.Scan(state)

	// Open the file, create a merkle hash.
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	info, err := file.Stat()
	if err != nil {
		return
	}
	merkle, err := MerkleFile(file, CalculateSegments(info.Size()))
	if err != nil {
		return
	}

	// Find a host.
	host, err := w.ChooseHost(state)
	for {
		if err != nil {
			return
		}
		if host.Frequency <= 100 && host.Tolerance < 10 {
			break
		}
		host, err = w.ChooseHost(state)
	}

	// Fill out the contract according to the whims of the host.
	fileContract := FileContract{
		ContractFund:       (host.Price + host.Burn) * 5000, // 5000 blocks.
		FileMerkleRoot:     merkle,
		FileSize:           uint64(info.Size()),
		Start:              state.Height() + 100,
		End:                state.Height() + 5100,
		ChallengeFrequency: host.Frequency,
		Tolerance:          host.Tolerance,
		ValidProofPayout:   host.Price,
		ValidProofAddress:  host.CoinAddress,
		MissedProofPayout:  host.Burn,
		// MissedProofAddress is going to be 0, funds sent to the burn address.
	}

	// Fund the client portion of the transaction.
	var t Transaction
	t.FileContracts = append(t.FileContracts, fileContract)
	err = w.FundTransaction(host.Price*5000, &t)
	if err != nil {
		return
	}

	// Send the contract to the host.

	// after getting a response, sign the reponse transaction and send the
	// signed transaction to the host along with the file itself.

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
