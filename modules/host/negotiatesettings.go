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
func (h *Host) capacity() (total, remaining uint64, err error) {
	// Total storage can be computed by summing the size of all the storage
	// folders.
	sfs, err := h.StorageFolders()
	if err != nil {
		return 0, 0, err
	}
	for _, sf := range sfs {
		total += sf.Capacity
		remaining += sf.CapacityRemaining
	}
	return total, remaining, nil
}

// managedRPCSettings is an rpc that returns the host's settings.
func (h *Host) managedRPCSettings(conn net.Conn) error {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateSettingsTime))

	var hes modules.HostExternalSettings
	var secretKey crypto.SecretKey
	err := func() error {
		h.mu.Lock()
		defer h.mu.Unlock()

		h.revisionNumber++
		secretKey = h.secretKey
		totalStorage, remainingStorage, err := h.capacity()
		if err != nil {
			return err
		}
		var netAddr modules.NetAddress
		if h.settings.NetAddress != "" {
			netAddr = h.settings.NetAddress
		} else {
			netAddr = h.autoAddress
		}
		hes = modules.HostExternalSettings{
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
		return nil
	}()
	if err != nil {
		return err
	}
	return crypto.WriteSignedObject(conn, hes, secretKey)
}
