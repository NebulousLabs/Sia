package host

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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

// listen listens for incoming RPCs and spawns an appropriate handler for each.
func (h *Host) listen() {
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			return
		}
		go h.handleConn(conn)
	}
}

// handleConn handles an incoming connection to the host, typically an RPC.
func (h *Host) handleConn(conn net.Conn) {
	defer conn.Close()
	// Set an initial duration that is generous, but finite. RPCs can extend
	// this if so desired.
	conn.SetDeadline(time.Now().Add(5 * time.Minute))

	var id types.Specifier
	if err := encoding.ReadObject(conn, &id, 16); err != nil {
		return
	}
	var err error
	switch id {
	case modules.RPCSettings:
		err = h.rpcSettings(conn)
	case modules.RPCUpload:
		err = h.rpcUpload(conn)
	case modules.RPCRenew:
		err = h.rpcRenew(conn)
	case modules.RPCRevise:
		err = h.rpcRevise(conn)
	case modules.RPCDownload:
		err = h.rpcDownload(conn)
	default:
		h.log.Printf("WARN: incoming conn %v requested unknown RPC \"%v\"", conn.RemoteAddr(), id)
		return
	}
	if err != nil {
		h.log.Printf("WARN: incoming RPC \"%v\" failed: %v", id, err)
	}
}

// rpcSettings is an rpc that returns the host's settings.
func (h *Host) rpcSettings(conn net.Conn) error {
	return encoding.WriteObject(conn, h.Settings())
}
