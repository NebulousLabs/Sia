package sia

import (
	"github.com/NebulousLabs/Andromeda/encoding"
)

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes that decode to the integer 1.
type HostAnnouncement struct {
	IPAddress             []byte
	MinFilesize           uint64
	MaxFilesize           uint64
	MinDuration           BlockHeight
	MaxDuration           BlockHeight
	MaxChallengeFrequency BlockHeight
	MinTolerance          uint64
	Price                 Currency
	Burn                  Currency
	CoinAddress           CoinAddress

	SpendConditions SpendConditions
	FreezeIndex     uint64 // The index of the output that froze coins.
}

// scanAndApplyHosts looks at the arbitrary data of a transaction and adds any
// hosts to the host database.
func (s *State) scanAndApplyHosts(t *Transaction) {
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
			Freeze:      Currency(ha.SpendConditions.TimeLock-s.Height()) * t.Outputs[ha.FreezeIndex].Value,
			CoinAddress: ha.CoinAddress,
		}
		if host.Freeze <= 0 {
			return
		}

		// Add the weight of the host to the total weight of the hosts in
		// the host database.
		s.HostList = append(s.HostList, host)
		s.TotalWeight += host.Weight()
	}
}
