package host

import (
	"net"

	"github.com/NebulousLabs/Sia/modules"
)

// initNetworking performs actions like port forwarding, and gets the host
// established on the network.
func (h *Host) initNetworking(address string) error {
	// Create listener and set address.
	var err error
	h.listener, err = net.Listen("tcp", address)
	if err != nil {
		return err
	}
	h.netAddress = modules.NetAddress(h.listener.Addr().String())

	// Networking subroutines.
	go h.forwardPort(h.netAddress.Port())
	go h.learnHostname()
	go h.listen()
	return nil
}
