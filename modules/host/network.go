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

	// Non-blocking, perform port forwarding and hostname discovery.
	go func() {
		h.resourceLock.RLock()
		defer h.resourceLock.RUnlock()

		// If the host has closed, return immediately.
		if h.closed {
			return
		}

		h.mu.RLock()
		port := h.netAddress.Port()
		h.mu.RUnlock()
		h.forwardPort(port)
		h.learnHostname()
	}()

	// Launch the listener.
	h.resourceLock.RLock()
	go h.threadedListen()
	return nil
}

// threadedHandleConn handles an incoming connection to the host, typically an
// RPC. threadedHandleConn is responsible for releasing a readlock on
// host.resourceLock when all communications have completed.
func (h *Host) threadedHandleConn(conn net.Conn) {
	// All threaded functions are called holding the close lock, and are
	// expected to keep it until they no longer need access to the host's
	// resources.
	defer h.resourceLock.RUnlock()
	// If the host resources are unavailable, return early.
	if h.closed {
		return
	}

	// Set an initial duration that is generous, but finite. RPCs can extend
	// this if desired.
	err := conn.SetDeadline(time.Now().Add(5 * time.Minute))
	if err != nil {
		h.log.Println("WARN: could not set deadline on connection:", err)
		return
	}
	defer conn.Close()

	// Read a specifier indicating which action is beeing called.
	var id types.Specifier
	if err := encoding.ReadObject(conn, &id, 16); err != nil {
		atomic.AddUint64(&h.atomicMalformedCalls, 1)
		h.log.Printf("WARN: incoming conn %v was malformed", conn.RemoteAddr())
		return
	}

	switch id {
	case modules.RPCDownload:
		atomic.AddUint64(&h.atomicDownloadCalls, 1)
		err = h.managedRPCDownload(conn)
	case modules.RPCRenew:
		atomic.AddUint64(&h.atomicRenewCalls, 1)
		err = h.managedRPCRenew(conn)
	case modules.RPCRevise:
		atomic.AddUint64(&h.atomicReviseCalls, 1)
		err = h.managedRPCRevise(conn)
	case modules.RPCSettings:
		atomic.AddUint64(&h.atomicSettingsCalls, 1)
		err = h.managedRPCSettings(conn)
	case modules.RPCUpload:
		atomic.AddUint64(&h.atomicUploadCalls, 1)
		err = h.managedRPCUpload(conn)
	default:
		atomic.AddUint64(&h.atomicErroredCalls, 1)
		h.log.Printf("WARN: incoming conn %v requested unknown RPC \"%v\"", conn.RemoteAddr(), id)
		return
	}
	if err != nil {
		atomic.AddUint64(&h.atomicErroredCalls, 1)
		h.log.Printf("WARN: incoming RPC \"%v\" failed: %v", id, err)
	}
}

// listen listens for incoming RPCs and spawns an appropriate handler for each.
func (h *Host) threadedListen() {
	// All threaded are called holding a readlock, and must release the
	// readlock upon terminating.
	defer h.resourceLock.RUnlock()

	// If the host has closed, some of the necessary resources will not be
	// available, and therefore the host should not be receiving connections.
	if h.closed {
		return
	}

	// Receive connections until an error is returned by the listener. When an
	// error is returned, there will be no more calls to receive.
	for {
		// Block until there is a connection to handle.
		conn, err := h.listener.Accept()
		if err != nil {
			return
		}

		// Grab the resource lock before creating a goroutine.
		h.resourceLock.RLock()
		go h.threadedHandleConn(conn)
	}
}

// managedRPCSettings is an rpc that returns the host's settings.
func (h *Host) managedRPCSettings(conn net.Conn) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return encoding.WriteObject(conn, h.Settings())
}
