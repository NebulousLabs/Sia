package siad

import (
	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/siacore"
)

type Renter struct {
	HostList    []Host
	TotalWeight siacore.Currency
}

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes that decode to the integer 1.
type HostAnnouncement struct {
	IPAddress             []byte
	MinFilesize           uint64
	MaxFilesize           uint64
	MinDuration           siacore.BlockHeight
	MaxDuration           siacore.BlockHeight
	MaxChallengeFrequency siacore.BlockHeight
	MinTolerance          uint64
	Price                 siacore.Currency
	Burn                  siacore.Currency
	CoinAddress           siacore.CoinAddress

	SpendConditions siacore.SpendConditions
	FreezeIndex     uint64 // The index of the output that froze coins.
}

// scanAndApplyHosts looks at the arbitrary data of a transaction and adds any
// hosts to the host database.
func (r *Renter) scanAndApplyHosts(t *siacore.Transaction) {
	// Check the arbitrary data of the transaction to fill out the host database.
	if len(t.ArbitraryData) < 8 {
		return
	}

	dataIndicator := encoding.DecUint64(t.ArbitraryData[0:8])
	if dataIndicator == 1 {
		var ha HostAnnouncement
		encoding.Unmarshal(t.ArbitraryData[1:], ha)

		// Verify that the spend condiitons match.
		if ha.SpendConditions.CoinAddress() != t.Outputs[ha.FreezeIndex].SpendHash {
			return
		}

		// Add the host to the host database.
		host := Host{
			IPAddress:   string(ha.IPAddress),
			MinSize:     ha.MinFilesize,
			MaxSize:     ha.MaxFilesize,
			Duration:    ha.MaxDuration,
			Frequency:   ha.MaxChallengeFrequency,
			Tolerance:   ha.MinTolerance,
			Price:       ha.Price,
			Burn:        ha.Burn,
			Freeze:      siacore.Currency(ha.SpendConditions.TimeLock-s.Height()) * t.Outputs[ha.FreezeIndex].Value,
			CoinAddress: ha.CoinAddress,
		}
		if host.Freeze <= 0 {
			return
		}

		// Add the weight of the host to the total weight of the hosts in
		// the host database.
		r.HostList = append(r.HostList, host)
		r.TotalWeight += host.Weight()
	}
}
