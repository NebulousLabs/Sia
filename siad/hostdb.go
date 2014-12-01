package siad

import (
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

type HostDatabase struct {
	State       *siacore.State
	HostList    []HostEntry
	TotalWeight siacore.Currency
}

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes that decode to the integer 1.
type HostAnnouncement struct {
	IPAddress             network.NetAddress
	MinFilesize           uint64
	MaxFilesize           uint64
	MinDuration           siacore.BlockHeight
	MaxDuration           siacore.BlockHeight
	MinChallengeFrequency siacore.BlockHeight
	MaxChallengeFrequency siacore.BlockHeight
	MinTolerance          uint64
	Price                 siacore.Currency
	Burn                  siacore.Currency
	CoinAddress           siacore.CoinAddress

	SpendConditions siacore.SpendConditions
	FreezeIndex     uint64 // The index of the output that froze coins.
}

// the Host struct is kept in the client package because it's what the client
// uses to weigh hosts and pick them out when storing files.
type HostEntry struct {
	IPAddress   network.NetAddress
	MinFilesize uint64
	MaxFilesize uint64
	MinDuration siacore.BlockHeight
	MaxDuration siacore.BlockHeight
	Frequency   siacore.BlockHeight
	Tolerance   uint64
	Price       siacore.Currency
	Burn        siacore.Currency
	Freeze      siacore.Currency
	CoinAddress siacore.CoinAddress
}

// host.Weight() determines the weight of a specific host.
func (h *HostEntry) Weight() siacore.Currency {
	return h.Freeze * h.Burn / h.Price
}

// scanAndApplyHosts looks at the arbitrary data of a transaction and adds any
// hosts to the host database.
func (hdb *HostDatabase) scanAndApplyHosts(t *siacore.Transaction) {
	// Check the arbitrary data of the transaction to fill out the host database.
	if len(t.ArbitraryData) < 8 {
		return
	}

	dataIndicator := encoding.DecUint64(t.ArbitraryData[0:8])
	if dataIndicator == 1 {
		var ha HostAnnouncement
		err := encoding.Unmarshal(t.ArbitraryData[1:], ha)
		if err != nil {
			return
		}

		// Verify that the host has declared values that are relevant to our
		// interests.
		if ha.SpendConditions.CoinAddress() != t.Outputs[ha.FreezeIndex].SpendHash {
			return
		}
		if ha.MaxChallengeFrequency > 100 {
			return
		}
		if ha.MinTolerance > 10 {
			return
		}
		freeze := siacore.Currency(ha.SpendConditions.TimeLock-hdb.State.Height()) * t.Outputs[ha.FreezeIndex].Value
		if freeze <= 0 {
			return
		}

		// Add the host to the host database.
		host := HostEntry{
			IPAddress:   ha.IPAddress,
			MinFilesize: ha.MinFilesize,
			MaxFilesize: ha.MaxFilesize,
			MinDuration: ha.MinDuration,
			MaxDuration: ha.MaxDuration,
			Frequency:   ha.MaxChallengeFrequency,
			Tolerance:   ha.MinTolerance,
			Price:       ha.Price,
			Burn:        ha.Burn,
			Freeze:      freeze,
			CoinAddress: ha.CoinAddress,
		}

		// Add the weight of the host to the total weight of the hosts in
		// the host database.
		hdb.HostList = append(hdb.HostList, host)
		hdb.TotalWeight += host.Weight()
	}
}

// ChooseHost orders the hosts by weight and picks one at random.
func (hdb *HostDatabase) ChooseHost(wallet *Wallet) (h HostEntry, err error) {
	if len(hdb.HostList) == 0 {
		err = errors.New("no hosts found")
		return
	}
	if hdb.TotalWeight == 0 {
		panic("state has 0 total weight but not 0 length host list?")
	}

	// Get a random number between 0 and state.TotalWeight and then scroll
	// through state.HostList until at least that much weight has been passed.
	randInt, err := rand.Int(rand.Reader, big.NewInt(int64(hdb.TotalWeight)))
	if err != nil {
		return
	}
	randWeight := siacore.Currency(randInt.Int64())
	weightPassed := siacore.Currency(0)
	var i int
	for i = 0; randWeight >= weightPassed; i++ {
		weightPassed += hdb.HostList[i].Weight()
	}

	h = hdb.HostList[i]
	return
}

func (e *Environment) SetHostSettings(ha HostAnnouncement) {
	e.host.settings = ha
}
