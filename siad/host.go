package siad

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/siacore"
)

const (
	HostAnnouncementPrefix = uint64(1)
)

type Host struct {
	Settings HostAnnouncement

	SpaceRemaining uint64
}

// SetHostSettings changes the settings according to the input. Need a setter
// because Environment.host is not exported.
func (e *Environment) SetHostSettings(ha HostAnnouncement) {
	e.host.Settings = ha
}

// Wallet.HostAnnounceSelf() creates a host announcement transaction, adding
// information to the arbitrary data and then signing the transaction.
func (e *Environment) HostAnnounceSelf(freezeVolume siacore.Currency, freezeUnlockHeight siacore.BlockHeight, minerFee siacore.Currency) (t siacore.Transaction, err error) {
	info := e.host.Settings

	// Fund the transaction.
	err = e.wallet.FundTransaction(freezeVolume+minerFee, &t)
	if err != nil {
		return
	}

	// Add the miner fee.
	t.MinerFees = append(t.MinerFees, minerFee)

	// Add the output with the freeze volume.
	freezeConditions := e.wallet.SpendConditions
	freezeConditions.TimeLock = freezeUnlockHeight
	info.FreezeIndex = uint64(len(t.Outputs))
	info.SpendConditions = freezeConditions
	t.Outputs = append(t.Outputs, siacore.Output{Value: freezeVolume, SpendHash: freezeConditions.CoinAddress()})

	// Frozen money can't currently be recovered.
	/*
		num, exists := w.OpenFreezeConditions[freezeUnlockHeight]
		if exists {
			w.OpenFreezeConditions[freezeUnlockHeight] = num + 1
		} else {
			w.OpenFreezeConditions[freezeUnlockHeight] = 1
		}
	*/

	// Add the announcement as arbitrary data.
	prefixBytes := encoding.Marshal(HostAnnouncementPrefix)
	announcementBytes := encoding.Marshal(info)
	t.ArbitraryData = append(prefixBytes, announcementBytes...)

	// Sign the transaction.
	e.wallet.SignTransaction(&t)

	err = e.AcceptTransaction(t)
	if err != nil {
		return
	}

	// TODO: Have a different method for setting max filesize.
	e.host.SpaceRemaining = e.host.Settings.MaxFilesize

	return
}

// considerContract takes a contract and verifies that the negotiations, such
// as price, tolerance, etc. are all valid within the host settings. If so,
// inputs are added to fund the burn part of the contract fund, then the
// updated contract is signed and returned.
func (e *Environment) considerContract(t siacore.Transaction) (nt siacore.Transaction, err error) {
	// Set the new transaction equal to the old transaction. Pretty sure that
	// go does not allow you to return the same variable that was used as
	// input. We could use a pointer, but that might be a bad idea. This call
	// is happening over the network anyway.
	nt = t

	contractDuration := nt.FileContracts[0].End - nt.FileContracts[0].Start // Duration according to the contract.
	fullDuration := nt.FileContracts[0].End - e.Height()                    // Duration that the host will actually be storing the file.
	fileSize := nt.FileContracts[0].FileSize

	// Check that there is only one file contract.
	if len(nt.FileContracts) != 1 {
		err = errors.New("will not accept a transaction with more than one file contract")
		return
	}

	// Check that the file size listed in the contract is in bounds.
	if fileSize < e.host.Settings.MinFilesize || fileSize > e.host.Settings.MaxFilesize {
		err = errors.New("file is of incorrect size")
		return
	}

	// Check that the duration of the contract is in bounds.
	if fullDuration < e.host.Settings.MinDuration || fullDuration < e.host.Settings.MinDuration {
		err = errors.New("contract duration is out of bounds")
		return
	}

	// Check that challenges will not be happening too frequently or infrequently.
	if nt.FileContracts[0].ChallengeFrequency < e.host.Settings.MaxChallengeFrequency || nt.FileContracts[0].ChallengeFrequency > e.host.Settings.MinChallengeFrequency {
		err = errors.New("challenges frequency is too often")
		return
	}

	// Check that tolerance is acceptible.
	if nt.FileContracts[0].Tolerance < e.host.Settings.MinTolerance {
		err = errors.New("tolerance is too low")
		return
	}

	// Outputs for successful proofs need to go to the correct address.
	if nt.FileContracts[0].ValidProofAddress != e.CoinAddress() {
		err = errors.New("coins are not paying out to correct address")
		return
	}

	// Output for failed proofs needs to be the 0 address.
	emptyAddress := siacore.CoinAddress{}
	if nt.FileContracts[0].MissedProofAddress != emptyAddress {
		err = errors.New("burn payout needs to go to the empty address")
		return
	}

	// Verify that output for failed proofs matches burn.
	requiredBurn := e.host.Settings.Burn * siacore.Currency(fileSize) * siacore.Currency(nt.FileContracts[0].ChallengeFrequency)
	if nt.FileContracts[0].MissedProofPayout > requiredBurn {
		err = errors.New("burn payout is too high for a missed proof.")
		return
	}

	// Verify that the outputs for successful proofs are high enough.
	requiredSize := e.host.Settings.Price * siacore.Currency(fileSize) * siacore.Currency(nt.FileContracts[0].ChallengeFrequency)
	if nt.FileContracts[0].ValidProofPayout < requiredSize {
		err = errors.New("valid proof payout is too low")
		return
	}

	// Verify that the contract fund covers the payout and burn for the whole
	// duration.
	requiredFund := (e.host.Settings.Burn + e.host.Settings.Price) * siacore.Currency(fileSize) * siacore.Currency(contractDuration)
	if nt.FileContracts[0].ContractFund < requiredFund {
		err = errors.New("ContractFund does not cover the entire duration of the contract.")
		return
	}

	// Add some inputs and outputs to the transaction to fund the burn half.
	e.wallet.FundTransaction(e.host.Settings.Burn*siacore.Currency(fileSize)*siacore.Currency(contractDuration), &nt)
	e.wallet.SignTransaction(&nt)

	return
}

func CreateHost() *Host {
	return new(Host)
}
