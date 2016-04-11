package host

import (
	"net"

	"github.com/NebulousLabs/Sia/crypto"
)

// managedRPCSettings is an rpc that returns the host's settings.
func (h *Host) managedRPCSettings(conn net.Conn) error {
	h.mu.RLock()
	settings := h.settings
	secretKey := h.secretKey
	h.mu.RUnlock()
	return crypto.WriteSignedObject(conn, settings, secretKey)
}
