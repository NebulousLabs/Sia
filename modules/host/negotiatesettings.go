package host

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// capacity returns the amount of storage still available on the machine. The
// amount can be negative if the total capacity was reduced to below the active
// capacity.
func (h *Host) capacity() (total uint64, remaining uint64) {
	// Total storage can be computed by summing the size of all the storage
	// folders.
	for _, sf := range h.storageFolders {
		total += sf.Size
		remaining += sf.SizeRemaining
	}
	return total, remaining
}

// managedRPCSettings is an rpc that returns the host's settings.
func (h *Host) managedRPCSettings(conn net.Conn) error {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateSettingsTime))

	h.mu.Lock()
	h.revisionNumber++
	secretKey := h.secretKey
	totalStorage, remainingStorage := h.capacity()
	var netAddr modules.NetAddress
	if h.settings.NetAddress != "" {
		netAddr = h.settings.NetAddress
	} else {
		netAddr = h.autoAddress
	}
	hes := modules.HostExternalSettings{
		AcceptingContracts: h.settings.AcceptingContracts,
		MaxBatchSize:       h.settings.MaxBatchSize,
		MaxDuration:        h.settings.MaxDuration,
		NetAddress:         netAddr,
		RemainingStorage:   remainingStorage,
		SectorSize:         modules.SectorSize,
		TotalStorage:       totalStorage,
		UnlockHash:         h.unlockHash,
		WindowSize:         h.settings.WindowSize,

		Collateral:            h.settings.Collateral,
		MaxCollateralFraction: h.settings.MaxCollateralFraction,
		MaxCollateral:         h.settings.MaxCollateral,

		ContractPrice:          h.settings.MinimumContractPrice,
		DownloadBandwidthPrice: h.settings.MinimumDownloadBandwidthPrice,
		StoragePrice:           h.settings.MinimumStoragePrice,
		UploadBandwidthPrice:   h.settings.MinimumUploadBandwidthPrice,

		RevisionNumber: h.revisionNumber,
		Version:        build.Version,
	}
	h.mu.Unlock()
	return crypto.WriteSignedObject(conn, hes, secretKey)
}
