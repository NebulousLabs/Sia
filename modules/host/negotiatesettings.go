package host

import (
	"net"
	"time"

	// "github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// managedRPCSettings is an rpc that returns the host's settings.
func (h *Host) managedRPCSettings(conn net.Conn) error {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateSettingsTime))

	/*
		h.mu.RLock()
		secretKey := h.secretKey
		hes := modules.HostExternalSettings{
			AcceptingContracts: h.settings.AcceptingContracts,
			MaxBatchSize:       h.settings.MaxBatchSize,
			MaxDuration:        h.settings.MaxDuration,
			NetAddress:         h.netAddress,
				RemainingStorage:
				SectorSize: modules.SectorSize,
				TotalStorage:
				UnlockHash:
				WindowSize:
		}
		h.mu.RUnlock()
		// return crypto.WriteSignedObject(conn, settings, secretKey)
	*/

	return nil
}
