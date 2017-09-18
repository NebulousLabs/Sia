package host

// TODO: seems like there would be problems with the negotiation protocols if
// the renter tried something like 'form' or 'renew' but then the connections
// dropped after the host completed the transaction but before the host was
// able to send the host signatures for the transaction.
//
// Especially on a renew, the host choosing to hold the renter signatures
// hostage could be a pretty significant problem, and would require the renter
// to attempt a double-spend to either force the transaction onto the
// blockchain or to make sure that the host cannot abscond with the funds
// without commitment.
//
// Incentive for the host to do such a thing is pretty low - they will still
// have to keep all the files following a renew in order to get the money.

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// rpcSettingsDeprecated is a specifier for a deprecated settings request.
var rpcSettingsDeprecated = types.Specifier{'S', 'e', 't', 't', 'i', 'n', 'g', 's'}

// threadedUpdateHostname periodically runs 'managedLearnHostname', which
// checks if the host's hostname has changed, and makes an updated host
// announcement if so.
func (h *Host) threadedUpdateHostname(closeChan chan struct{}) {
	defer close(closeChan)
	for {
		h.managedLearnHostname()
		// Wait 30 minutes to check again. If the hostname is changing
		// regularly (more than once a week), we want the host to be able to be
		// seen as having 95% uptime. Every minute that the announcement is
		// pointing to the wrong address is a minute of perceived downtime to
		// the renters.
		select {
		case <-h.tg.StopChan():
			return
		case <-time.After(time.Minute * 30):
			continue
		}
	}
}

// threadedTrackWorkingStatus periodically checks if the host is working,
// where working is defined as having received 3 settings calls in the past 15
// minutes.
func (h *Host) threadedTrackWorkingStatus(closeChan chan struct{}) {
	defer close(closeChan)

	// Before entering the longer loop, try a greedy, faster attempt to verify
	// that the host is working.
	prevSettingsCalls := atomic.LoadUint64(&h.atomicSettingsCalls)
	select {
	case <-h.tg.StopChan():
		return
	case <-time.After(workingStatusFirstCheck):
	}
	settingsCalls := atomic.LoadUint64(&h.atomicSettingsCalls)

	// sanity check
	if prevSettingsCalls > settingsCalls {
		build.Severe("the host's settings calls decremented")
	}

	h.mu.Lock()
	if settingsCalls-prevSettingsCalls >= workingStatusThreshold {
		h.workingStatus = modules.HostWorkingStatusWorking
	}
	// First check is quick, don't set to 'not working' if host has not been
	// contacted enough times.
	h.mu.Unlock()

	for {
		prevSettingsCalls = atomic.LoadUint64(&h.atomicSettingsCalls)
		select {
		case <-h.tg.StopChan():
			return
		case <-time.After(workingStatusFrequency):
		}
		settingsCalls = atomic.LoadUint64(&h.atomicSettingsCalls)

		// sanity check
		if prevSettingsCalls > settingsCalls {
			build.Severe("the host's settings calls decremented")
			continue
		}

		h.mu.Lock()
		if settingsCalls-prevSettingsCalls >= workingStatusThreshold {
			h.workingStatus = modules.HostWorkingStatusWorking
		} else {
			h.workingStatus = modules.HostWorkingStatusNotWorking
		}
		h.mu.Unlock()
	}
}

// threadedTrackConnectabilityStatus periodically checks if the host is
// connectable at its netaddress.
func (h *Host) threadedTrackConnectabilityStatus(closeChan chan struct{}) {
	defer close(closeChan)

	// Wait briefly before checking the first time. This gives time for any port
	// forwarding to complete.
	select {
	case <-h.tg.StopChan():
		return
	case <-time.After(connectabilityCheckFirstWait):
	}

	for {
		h.mu.RLock()
		autoAddr := h.autoAddress
		userAddr := h.settings.NetAddress
		h.mu.RUnlock()

		activeAddr := autoAddr
		if userAddr != "" {
			activeAddr = userAddr
		}

		dialer := &net.Dialer{
			Cancel:  h.tg.StopChan(),
			Timeout: connectabilityCheckTimeout,
		}
		conn, err := dialer.Dial("tcp", string(activeAddr))

		var status modules.HostConnectabilityStatus
		if err != nil {
			status = modules.HostConnectabilityStatusNotConnectable
		} else {
			conn.Close()
			status = modules.HostConnectabilityStatusConnectable
		}
		h.mu.Lock()
		h.connectabilityStatus = status
		h.mu.Unlock()

		select {
		case <-h.tg.StopChan():
			return
		case <-time.After(connectabilityCheckFrequency):
		}
	}
}

// initNetworking performs actions like port forwarding, and gets the
// host established on the network.
func (h *Host) initNetworking(address string) (err error) {
	// Create the listener and setup the close procedures.
	h.listener, err = h.dependencies.listen("tcp", address)
	if err != nil {
		return err
	}
	// Automatically close the listener when h.tg.Stop() is called.
	threadedListenerClosedChan := make(chan struct{})
	h.tg.OnStop(func() {
		err := h.listener.Close()
		if err != nil {
			h.log.Println("WARN: closing the listener failed:", err)
		}

		// Wait until the threadedListener has returned to continue shutdown.
		<-threadedListenerClosedChan
	})

	// Set the initial working state of the host
	h.workingStatus = modules.HostWorkingStatusChecking

	// Set the initial connectability state of the host
	h.connectabilityStatus = modules.HostConnectabilityStatusChecking

	// Set the port.
	_, port, err := net.SplitHostPort(h.listener.Addr().String())
	if err != nil {
		return err
	}
	h.port = port
	if build.Release == "testing" {
		// Set the autoAddress to localhost for testing builds only.
		h.autoAddress = modules.NetAddress(net.JoinHostPort("localhost", h.port))
	}

	// Non-blocking, perform port forwarding and create the hostname discovery
	// thread.
	go func() {
		// Add this function to the threadgroup, so that the logger will not
		// disappear before port closing can be registered to the threadgrourp
		// OnStop functions.
		err := h.tg.Add()
		if err != nil {
			// If this goroutine is not run before shutdown starts, this
			// codeblock is reachable.
			return
		}
		defer h.tg.Done()

		err = h.managedForwardPort(port)
		if err != nil {
			h.log.Println("ERROR: failed to forward port:", err)
		} else {
			// Clear the port that was forwarded at startup.
			h.tg.OnStop(func() {
				err := h.managedClearPort()
				if err != nil {
					h.log.Println("ERROR: failed to clear port:", err)
				}
			})
		}

		threadedUpdateHostnameClosedChan := make(chan struct{})
		go h.threadedUpdateHostname(threadedUpdateHostnameClosedChan)
		h.tg.OnStop(func() {
			<-threadedUpdateHostnameClosedChan
		})

		threadedTrackWorkingStatusClosedChan := make(chan struct{})
		go h.threadedTrackWorkingStatus(threadedTrackWorkingStatusClosedChan)
		h.tg.OnStop(func() {
			<-threadedTrackWorkingStatusClosedChan
		})

		threadedTrackConnectabilityStatusClosedChan := make(chan struct{})
		go h.threadedTrackConnectabilityStatus(threadedTrackConnectabilityStatusClosedChan)
		h.tg.OnStop(func() {
			<-threadedTrackConnectabilityStatusClosedChan
		})
	}()

	// Launch the listener.
	go h.threadedListen(threadedListenerClosedChan)
	return nil
}

// threadedHandleConn handles an incoming connection to the host, typically an
// RPC.
func (h *Host) threadedHandleConn(conn net.Conn) {
	err := h.tg.Add()
	if err != nil {
		return
	}
	defer h.tg.Done()

	// Close the conn on host.Close or when the method terminates, whichever comes
	// first.
	connCloseChan := make(chan struct{})
	defer close(connCloseChan)
	go func() {
		select {
		case <-h.tg.StopChan():
		case <-connCloseChan:
		}
		conn.Close()
	}()

	// Set an initial duration that is generous, but finite. RPCs can extend
	// this if desired.
	err = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	if err != nil {
		h.log.Println("WARN: could not set deadline on connection:", err)
		return
	}

	// Read a specifier indicating which action is being called.
	var id types.Specifier
	if err := encoding.ReadObject(conn, &id, 16); err != nil {
		atomic.AddUint64(&h.atomicUnrecognizedCalls, 1)
		h.log.Debugf("WARN: incoming conn %v was malformed: %v", conn.RemoteAddr(), err)
		return
	}

	switch id {
	case modules.RPCDownload:
		atomic.AddUint64(&h.atomicDownloadCalls, 1)
		err = extendErr("incoming RPCDownload failed: ", h.managedRPCDownload(conn))
	case modules.RPCRenewContract:
		atomic.AddUint64(&h.atomicRenewCalls, 1)
		err = extendErr("incoming RPCRenewContract failed: ", h.managedRPCRenewContract(conn))
	case modules.RPCFormContract:
		atomic.AddUint64(&h.atomicFormContractCalls, 1)
		err = extendErr("incoming RPCFormContract failed: ", h.managedRPCFormContract(conn))
	case modules.RPCReviseContract:
		atomic.AddUint64(&h.atomicReviseCalls, 1)
		err = extendErr("incoming RPCReviseContract failed: ", h.managedRPCReviseContract(conn))
	case modules.RPCSettings:
		atomic.AddUint64(&h.atomicSettingsCalls, 1)
		err = extendErr("incoming RPCSettings failed: ", h.managedRPCSettings(conn))
	case rpcSettingsDeprecated:
		h.log.Debugln("Received deprecated settings call")
	default:
		h.log.Debugf("WARN: incoming conn %v requested unknown RPC \"%v\"", conn.RemoteAddr(), id)
		atomic.AddUint64(&h.atomicUnrecognizedCalls, 1)
	}
	if err != nil {
		atomic.AddUint64(&h.atomicErroredCalls, 1)
		err = extendErr("error with "+conn.RemoteAddr().String()+": ", err)
		h.managedLogError(err)
	}
}

// listen listens for incoming RPCs and spawns an appropriate handler for each.
func (h *Host) threadedListen(closeChan chan struct{}) {
	defer close(closeChan)

	// Receive connections until an error is returned by the listener. When an
	// error is returned, there will be no more calls to receive.
	for {
		// Block until there is a connection to handle.
		conn, err := h.listener.Accept()
		if err != nil {
			return
		}

		go h.threadedHandleConn(conn)

		// Soft-sleep to ratelimit the number of incoming connections.
		select {
		case <-h.tg.StopChan():
		case <-time.After(rpcRatelimit):
		}
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

// NetworkMetrics returns information about the types of rpc calls that have
// been made to the host.
func (h *Host) NetworkMetrics() modules.HostNetworkMetrics {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return modules.HostNetworkMetrics{
		DownloadCalls:     atomic.LoadUint64(&h.atomicDownloadCalls),
		ErrorCalls:        atomic.LoadUint64(&h.atomicErroredCalls),
		FormContractCalls: atomic.LoadUint64(&h.atomicFormContractCalls),
		RenewCalls:        atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:       atomic.LoadUint64(&h.atomicReviseCalls),
		SettingsCalls:     atomic.LoadUint64(&h.atomicSettingsCalls),
		UnrecognizedCalls: atomic.LoadUint64(&h.atomicUnrecognizedCalls),
	}
}
