package host

import (
	"net"
	"sync/atomic"
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

// threadedHandleConn handles an incoming connection to the host, typically an RPC.
func (h *Host) threadedHandleConn(conn net.Conn) {
	// Set an initial duration that is generous, but finite. RPCs can extend
	// this if so desired.
	conn.SetDeadline(time.Now().Add(5 * time.Minute))
	defer conn.Close()

	// Read a specifier indicating which action is beeing called.
	var id types.Specifier
	if err := encoding.ReadObject(conn, &id, 16); err != nil {
		atomic.AddUint64(&h.malformedCalls, 1)
		h.log.Printf("WARN: incoming conn %v was malformed", conn.RemoteAddr())
		return
	}

	var err error
	switch id {
	case modules.RPCDownload:
		atomic.AddUint64(&h.downloadCalls, 1)
		err = h.rpcDownload(conn)
	case modules.RPCRenew:
		atomic.AddUint64(&h.renewCalls, 1)
		err = h.rpcRenew(conn)
	case modules.RPCRevise:
		atomic.AddUint64(&h.reviseCalls, 1)
		err = h.rpcRevise(conn)
	case modules.RPCSettings:
		atomic.AddUint64(&h.settingsCalls, 1)
		err = h.rpcSettings(conn)
	case modules.RPCUpload:
		atomic.AddUint64(&h.uploadCalls, 1)
		err = h.threadedRPCUpload(conn)
	default:
		atomic.AddUint64(&h.erroredCalls, 1)
		h.log.Printf("WARN: incoming conn %v requested unknown RPC \"%v\"", conn.RemoteAddr(), id)
		return
	}
	if err != nil {
		atomic.AddUint64(&h.erroredCalls, 1)
		h.log.Printf("WARN: incoming RPC \"%v\" failed: %v", id, err)
	}
}

// listen listens for incoming RPCs and spawns an appropriate handler for each.
func (h *Host) listen() {
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			return
		}
		go h.threadedHandleConn(conn)
	}
}

// rpcSettings is an rpc that returns the host's settings.
func (h *Host) rpcSettings(conn net.Conn) error {
	return encoding.WriteObject(conn, h.Settings())
}
