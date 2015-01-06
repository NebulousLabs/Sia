package host

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/sia/hostdb"
)

// HostAnnounceSelf creates a host announcement transaction, adding
// information to the arbitrary data and then signing the transaction.
func (bh *BasicHost) AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (t consensus.Transaction, err error) {
	// Get the encoded announcement based on the host settings.
	bh.RLock()
	info := bh.Settings
	bh.RUnlock()
	announcement := string(encoding.MarshalAll(hostdb.HostAnnouncementPrefix, info))

	// Fill out the transaction.
	id, err := bh.Wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	err = bh.Wallet.FundTransaction(id, freezeVolume)
	if err != nil {
		return
	}
	info.SpendConditions, info.FreezeIndex, err = bh.Wallet.AddTimelockedRefund(id, freezeVolume, freezeUnlockHeight)
	if err != nil {
		return
	}
	err = bh.Wallet.AddArbitraryData(id, announcement)
	if err != nil {
		return
	}
	// TODO: Have the wallet manually add a fee? How should this be managed?
	t, err = bh.Wallet.SignTransaction(id, true)
	if err != nil {
		return
	}

	return
}
