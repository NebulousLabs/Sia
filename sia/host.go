package sia

const (
	HostAnnouncementPrefix = uint64(1)
)

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes matching the numerical integer '1'.
type HostAnnouncement struct {
	IPAddress             []byte
	MinFilesize           uint64
	MaxFilesize           uint64
	MaxDuration           BlockHeight
	MaxChallengeFrequency BlockHeight
	MinTolerance          uint64
	Price                 Currency
	Burn                  Currency
	CoinAddress           CoinAddress

	SpendConditions SpendConditions
	FreezeIndex     uint64
}

// Wallet.HostAnnounceSelf() creates a host announcement transaction, adding
// information to the arbitrary data and then signing the transaction.
func (w *Wallet) HostAnnounceSelf(info HostAnnouncement, freezeVolume Currency, freezeUnlockHeight BlockHeight, minerFee Currency, state *State) (t Transaction, err error) {
	w.Scan(state)

	// Fund the transaction.
	err = w.FundTransaction(freezeVolume+minerFee, &t)
	if err != nil {
		return
	}

	// Add the miner fee.
	t.MinerFees = append(t.MinerFees, minerFee)

	// Add the output with the freeze volume.
	freezeConditions := w.FreezeConditions(freezeUnlockHeight)
	t.Outputs = append(t.Outputs, Output{Value: freezeVolume, SpendHash: freezeConditions.CoinAddress()})
	num, exists := w.OpenFreezeConditions[freezeUnlockHeight]
	if exists {
		w.OpenFreezeConditions[freezeUnlockHeight] = num + 1
	} else {
		w.OpenFreezeConditions[freezeUnlockHeight] = 1
	}
	info.SpendConditions = freezeConditions

	// Add the announcement as arbitrary data.
	prefixBytes := Marshal(HostAnnouncementPrefix)
	announcementBytes := Marshal(info)
	t.ArbitraryData = append(prefixBytes, announcementBytes...)

	err = state.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}
