package host

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// threadedUpdateHostname periodically runs 'managedLearnHostname', which
// checks if the host's hostname has changed, and makes an updated host
// announcement if so.
//
// TODO: This thread doesn't actually have clean shutdown, because the sleep is
// outside of of the resource lock.
func (h *Host) threadedUpdateHostname() {
	for {
		h.resourceLock.RLock()
		if h.closed {
			// The host is closed, the goroutine can exit.
			h.resourceLock.RUnlock()
			break
		}
		h.managedLearnHostname()
		h.resourceLock.RUnlock()

		// Wait 30 minutes to check again. If the hostname is changing
		// regularly (more than once a week), we want the host to be able to be
		// seen as having 95% uptime. Every minute that the announcement is
		// pointing to the wrong address is a minute of perceived downtime to
		// the renters.
		time.Sleep(time.Minute * 30)
	}
}

// initNetworking performs actions like port forwarding, and gets the host
// established on the network.
func (h *Host) initNetworking(address string) (err error) {
	// Create listener and set address.
	h.listener, err = h.dependencies.listen("tcp", address)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.port = modules.NetAddress(h.listener.Addr().String()).Port()
	if build.Release == "testing" {
		h.autoAddress = modules.NetAddress(net.JoinHostPort("localhost", h.port))
	}
	h.mu.Unlock()

	// Non-blocking, perform port forwarding and hostname discovery.
	go func() {
		h.resourceLock.RLock()
		defer h.resourceLock.RUnlock()
		if h.closed {
			return
		}

		err := h.managedForwardPort()
		if err != nil {
			h.log.Println("ERROR: failed to forward port:", err)
		}

		// Spin up the hostname checker. The hostname checker should not be
		// spun up until after the port has been forwarded, because it can
		// result in announcements being submitted to the blockchain.
		go h.threadedUpdateHostname()
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
	*/
	case modules.RPCFormContract:
		atomic.AddUint64(&h.atomicFormContractCalls, 1)
		err = h.managedRPCFormContract(conn)
	case modules.RPCReviseContract:
		atomic.AddUint64(&h.atomicReviseCalls, 1)
		err = h.managedRPCReviseContract(conn)
	case modules.RPCRevisionRequest:
		atomic.AddUint64(&h.atomicRevisionRequestCalls, 1)
		_, err = h.managedRPCRevisionRequest(conn)
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
//
// TODO: Does not seem like this function ever actually lets go of the resource
// lock.
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

// NetAddress returns the address at which the host can be reached.
func (h *Host) NetAddress() modules.NetAddress {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.settings.NetAddress != "" {
		return h.settings.NetAddress
	}
	return h.autoAddress
}

// RPCMetrics returns information about the types of rpc calls that have been
// made to the host.
func (h *Host) RPCMetrics() modules.HostRPCMetrics {
	return modules.HostRPCMetrics{
		DownloadCalls:     atomic.LoadUint64(&h.atomicDownloadCalls),
		ErrorCalls:        atomic.LoadUint64(&h.atomicErroredCalls),
		FormContractCalls: atomic.LoadUint64(&h.atomicFormContractCalls),
		RenewCalls:        atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:       atomic.LoadUint64(&h.atomicReviseCalls),
		SettingsCalls:     atomic.LoadUint64(&h.atomicSettingsCalls),
		UnrecognizedCalls: atomic.LoadUint64(&h.atomicUnrecognizedCalls),
	}
}
