package siad

import (
	"os"

	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/siacore"
)

type Renter struct {
	state  *siacore.State
	hostdb HostDatabase
}

// the Host struct is kept in the client package because it's what the client
// uses to weigh hosts and pick them out when storing files.
type Host struct {
	IPAddress   string
	MinSize     uint64
	MaxSize     uint64
	Duration    siacore.BlockHeight
	Frequency   siacore.BlockHeight
	Tolerance   uint64
	Price       siacore.Currency
	Burn        siacore.Currency
	Freeze      siacore.Currency
	CoinAddress siacore.CoinAddress
}

// host.Weight() determines the weight of a specific host.
func (h *Host) Weight() siacore.Currency {
	return h.Freeze * h.Burn / h.Price
}

// Wallet.ClientFundFileContract() takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (r *Renter) ClientProposeContract(filename string, wallet *Wallet) (err error) {
	// Scan the blockchain for outputs.
	wallet.Scan(r.state)

	// Open the file, create a merkle hash.
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	info, err := file.Stat()
	if err != nil {
		return
	}
	segments, err := hash.CalculateSegments(info.Size())
	if err != nil {
		return
	}
	merkle, err := hash.ReaderMerkleRoot(file, segments)
	if err != nil {
		return
	}

	// Find a host.
	host, err := r.hostdb.ChooseHost(wallet)
	for {
		if err != nil {
			return
		}
		host, err = r.hostdb.ChooseHost(wallet)
	}

	// Fill out the contract according to the whims of the host.
	fileContract := siacore.FileContract{
		ContractFund:       (host.Price + host.Burn) * 5000, // 5000 blocks.
		FileMerkleRoot:     merkle,
		FileSize:           uint64(info.Size()),
		Start:              r.state.Height() + 100,
		End:                r.state.Height() + 5100,
		ChallengeFrequency: host.Frequency,
		Tolerance:          host.Tolerance,
		ValidProofPayout:   host.Price,
		ValidProofAddress:  host.CoinAddress,
		MissedProofPayout:  host.Burn,
		// MissedProofAddress is going to be 0, funds sent to the burn address.
	}

	// Fund the client portion of the transaction.
	var t siacore.Transaction
	t.FileContracts = append(t.FileContracts, fileContract)
	err = wallet.FundTransaction(host.Price*5000, &t)
	if err != nil {
		return
	}

	// Send the contract to the host.

	// after getting a response, sign the reponse transaction and send the
	// signed transaction to the host along with the file itself.

	return
}
