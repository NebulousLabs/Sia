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
	h.listener, err = h.dependencies.listen("tcp", address)
	if err != nil {
		return err
	}
	h.netAddress = modules.NetAddress(h.listener.Addr().String())

	// Non-blocking, perform port forwarding and hostname discovery.
	go func() {
		h.resourceLock.RLock()
		defer h.resourceLock.RUnlock()
		if h.closed {
			return
		}

		h.mu.RLock()
		port := h.netAddress.Port()
		h.mu.RUnlock()
		err := h.forwardPort(port)
		if err != nil {
			h.log.Println("ERROR: failed to forward port:", err)
		}
		h.learnHostname()
	}()

	// Launch the listener.
	go h.threadedListen()
	return nil
}

// threadedHandleConn handles an incoming connection to the host, typically an
// RPC.
func (h *Host) threadedHandleConn(conn net.Conn) {
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
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
		atomic.AddUint64(&h.atomicUnrecognizedCalls, 1)
		h.log.Debugf("WARN: incoming conn %v was malformed: %v", conn.RemoteAddr(), err)
		return
	}

	switch id {
	/*
		case modules.RPCDownload:
			atomic.AddUint64(&h.atomicDownloadCalls, 1)
			err = h.managedRPCDownload(conn)
		case modules.RPCRenew:
			atomic.AddUint64(&h.atomicRenewCalls, 1)
			err = h.managedRPCRenew(conn)
		case modules.RPCUpload:
			atomic.AddUint64(&h.atomicUploadCalls, 1)
			err = h.managedRPCUpload(conn)
	*/
	case modules.RPCFormContract:
		atomic.AddUint64(&h.atomicFormContractCalls, 1)
		err = h.managedRPCFormContract(conn)
	case modules.RPCReviseContract:
		atomic.AddUint64(&h.atomicReviseCalls, 1)
		err = h.managedRPCReviseContract(conn)
	case modules.RPCSettings:
		atomic.AddUint64(&h.atomicSettingsCalls, 1)
		err = h.managedRPCSettings(conn)

	default:
		atomic.AddUint64(&h.atomicUnrecognizedCalls, 1)
		h.log.Debugf("WARN: incoming conn %v requested unknown RPC \"%v\"", conn.RemoteAddr(), id)
	}
	if err != nil {
		atomic.AddUint64(&h.atomicErroredCalls, 1)

		// If there have been less than 1000 errored rpcs, print the error
		// message. This is to help developers debug live systems that are
		// running into issues. Ultimately though, this error can be triggered
		// by a malicious actor, and therefore should not be logged except for
		// DEBUG builds.
		//
		// TODO: After the upgraded renter-host have more maturity, the
		// non-debug log call can be removed.
		erroredCalls := atomic.LoadUint64(&h.atomicErroredCalls)
		if erroredCalls < 1e3 {
			h.log.Printf("WARN: incoming RPC \"%v\" failed: %v", id, err)
		} else {
			h.log.Debugf("WARN: incoming RPC \"%v\" failed: %v", id, err)
		}
	}
}

// listen listens for incoming RPCs and spawns an appropriate handler for each.
func (h *Host) threadedListen() {
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
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
		go h.threadedHandleConn(conn)
	}
}
