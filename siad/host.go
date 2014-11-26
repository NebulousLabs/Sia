package siad

/*
import (
	"errors"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/siacore"
)

const (
	HostAnnouncementPrefix = uint64(1)
)


type Host struct {
	state *siacore.State

	Settings HostAnnouncement
}

// Wallet.HostAnnounceSelf() creates a host announcement transaction, adding
// information to the arbitrary data and then signing the transaction.
func (h *Host) HostAnnounceSelf(freezeVolume siacore.Currency, freezeUnlockHeight siacore.BlockHeight, minerFee siacore.Currency) (t siacore.Transaction, err error) {
	info := host.Settings

	// Fund the transaction.
	err = w.FundTransaction(freezeVolume+minerFee, &t)
	if err != nil {
		return
	}

	// Add the miner fee.
	t.MinerFees = append(t.MinerFees, minerFee)

	// Add the output with the freeze volume.
	freezeConditions := w.FreezeConditions(freezeUnlockHeight)
	t.Outputs = append(t.Outputs, siacore.Output{Value: freezeVolume, SpendHash: freezeConditions.CoinAddress()})
	num, exists := w.OpenFreezeConditions[freezeUnlockHeight]
	if exists {
		w.OpenFreezeConditions[freezeUnlockHeight] = num + 1
	} else {
		w.OpenFreezeConditions[freezeUnlockHeight] = 1
	}
	info.SpendConditions = freezeConditions
	info.FreezeIndex = 0

	// Add the announcement as arbitrary data.
	prefixBytes := encoding.Marshal(HostAnnouncementPrefix)
	announcementBytes := encoding.Marshal(info)
	t.ArbitraryData = append(prefixBytes, announcementBytes...)

	err = h.state.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}

func (h *Host) ConsiderContract(t siacore.Transaction) (nt siacore.Transaction, err error) {
	// Set the new transaction equal to the old transaction. Pretty sure that
	// go does not allow you to return the same variable that was used as
	// input. We could use a pointer, but that might be a bad idea. This call
	// is happening over the network anyway.
	nt = t

	// Check that there is only one file contract.
	if len(nt.FileContracts) != 1 {
		err = errors.New("will not accept a transaction with more than one file contract")
		return
	}

	// Verify that the client has put in the correct number of funds.

	// Check that the file size listed in the contract is in bounds.
	if nt.FileContracts[0].FileSize < w.HostSettings.MinFilesize || nt.FileContracts[0].FileSize > w.HostSettings.MaxFilesize {
		err = errors.New("file is of incorrect size")
		return
	}

	// Check that the duration of the contract is in bounds.
	currentHeight := siacore.BlockHeight(0) // GET THE CURRENT HEIGHT OF THE STATE AND PUT IT HERE.
	if nt.FileContracts[0].End-currentHeight < w.HostSettings.MinDuration || nt.FileContracts[0].End-currentHeight < w.HostSettings.MinDuration {
		err = errors.New("contract duration is out of bounds")
		return
	}

	// Check that challenges will not be happening too frequently.
	if nt.FileContracts[0].ChallengeFrequency < w.HostSettings.MaxChallengeFrequency {
		err = errors.New("challenges frequency is too often")
		return
	}

	// Check that tolerance is acceptible.
	if nt.FileContracts[0].Tolerance < w.HostSettings.MinTolerance {
		err = errors.New("tolerance is too low")
		return
	}

	// Outputs for successful proofs need to be appropriate.
	if nt.FileContracts[0].ValidProofAddress != w.SpendConditions.CoinAddress() {
		err = errors.New("coins are not paying out to correct address")
		return
	}

	// Output for failed proofs needs to be the 0 address.
	emptyAddress := siacore.CoinAddress{}
	if nt.FileContracts[0].MissedProofAddress != emptyAddress {
		err = errors.New("burn payout needs to go to the empty address")
		return
	}

	// Add some inputs and outputs to the transaction to fund the burn half.

	return
}

func CreateHost(s *siacore.State) *Host {
	return &Host{
		state: s,
	}
}
*/
