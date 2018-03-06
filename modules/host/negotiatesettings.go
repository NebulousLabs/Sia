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
	// Increment the revision number for the external settings
	h.revisionNumber++

	totalStorage, remainingStorage := h.capacity()
	var netAddr modules.NetAddress
	if h.settings.NetAddress != "" {
		netAddr = h.settings.NetAddress
	} else {
		netAddr = h.autoAddress
	}

	// Calculate contract price
	_, maxFee := h.tpool.FeeEstimation()
	contractPrice := maxFee.Mul64(10e3) // estimated size of txns host needs to fund
	if contractPrice.Cmp(h.settings.MinContractPrice) < 0 {
		contractPrice = h.settings.MinContractPrice
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

		ContractPrice:          contractPrice,
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

	// The revision number is updated so that the renter can be certain that
	// they have the most recent copy of the settings. The revision number and
	// signature can be compared against other settings objects that the renter
	// may have, and if the new revision number is not higher the renter can
	// suspect foul play. Largely, the revision number is in place to enable
	// renters to share host settings with each other, a feature that has not
	// yet been implemented.
	//
	// While updating the revision number, also grab the secret key and
	// external settings.
	var hes modules.HostExternalSettings
	var secretKey crypto.SecretKey
	h.mu.Lock()
	secretKey = h.secretKey
	hes = h.externalSettings()
	h.mu.Unlock()

	// Write the settings to the renter. If the write fails, return a
	// connection error.
	err := crypto.WriteSignedObject(conn, hes, secretKey)
	if err != nil {
		return ErrorConnection("failed WriteSignedObject during RPCSettings: " + err.Error())
	}
	return nil
}
