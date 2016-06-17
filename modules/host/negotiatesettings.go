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
func (h *Host) capacity() (total, remaining uint64) {
	// Total storage can be computed by summing the size of all the storage
	// folders.
	sfs := h.StorageFolders()
	for _, sf := range sfs {
		total += sf.Capacity
		remaining += sf.CapacityRemaining
	}
	return total, remaining
}

// externalSettings compiles and returns the external settings for the host.
func (h *Host) externalSettings() modules.HostExternalSettings {
	totalStorage, remainingStorage := h.capacity()
	var netAddr modules.NetAddress
	if h.settings.NetAddress != "" {
		netAddr = h.settings.NetAddress
	} else {
		netAddr = h.autoAddress
	}
	return modules.HostExternalSettings{
		AcceptingContracts:   h.settings.AcceptingContracts,
		MaxDownloadBatchSize: h.settings.MaxDownloadBatchSize,
		MaxDuration:          h.settings.MaxDuration,
		MaxReviseBatchSize:   h.settings.MaxReviseBatchSize,
		NetAddress:           netAddr,
		RemainingStorage:     remainingStorage,
		SectorSize:           modules.SectorSize,
		TotalStorage:         totalStorage,
		UnlockHash:           h.unlockHash,
		WindowSize:           h.settings.WindowSize,

		Collateral:    h.settings.Collateral,
		MaxCollateral: h.settings.MaxCollateral,

		ContractPrice:          h.settings.MinContractPrice,
		DownloadBandwidthPrice: h.settings.MinDownloadBandwidthPrice,
		StoragePrice:           h.settings.MinStoragePrice,
		UploadBandwidthPrice:   h.settings.MinUploadBandwidthPrice,

		RevisionNumber: h.revisionNumber,
		Version:        build.Version,
	}
}

// managedRPCSettings is an rpc that returns the host's settings.
func (h *Host) managedRPCSettings(conn net.Conn) error {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateSettingsTime))

	var hes modules.HostExternalSettings
	var secretKey crypto.SecretKey
	h.mu.Lock()
	h.revisionNumber++
	secretKey = h.secretKey
	hes = h.externalSettings()
	h.mu.Unlock()
	return crypto.WriteSignedObject(conn, hes, secretKey)
}
