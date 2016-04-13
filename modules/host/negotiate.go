package host

import (
	"net"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// rejectNegotiation will write a rejection response to the connection and
// return the input error composed with the error received from writing to the
// connection.
func rejectNegotiation(conn net.Conn, err error) error {
	writeErr := encoding.WriteObject(conn, err.Error())
	return composeErrors(err, writeErr)
}

// NetAddress returns the address at which the host can be reached.
func (h *Host) NetAddress() modules.NetAddress {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.netAddress
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
