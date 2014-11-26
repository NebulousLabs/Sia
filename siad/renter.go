package siad

/*
import (
	"os"

	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/siacore"
)

type Renter struct {
	state *siacore.State

	hostdb HostDatabase
}

// Wallet.ClientFundFileContract() takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (r *Renter) ClientProposeContract(state *siacore.State, filename string, wallet *Wallet) (err error) {
	// Scan the blockchain for outputs.
	wallet.Scan()

	// Open the file, create a merkle hash.
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	info, err := file.Stat()
	if err != nil {
		return
	}
	merkle, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(uint64(info.Size())))
	if err != nil {
		return
	}

	// Find a host.
	host, err := r.hostdb.ChooseHost(wallet)
	if err != nil {
		return
	}

	// Fill out the contract according to the whims of the host.
	fileContract := siacore.FileContract{
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

func CreateRenter(s *siacore.State) *Renter {
	return &Renter{
		state: s,
	}
}
*/
