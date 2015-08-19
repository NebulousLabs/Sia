package host

import (
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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

// TODO: maintain compatibility
func (h *Host) handleConn(conn net.Conn) {
	defer conn.Close()

	var id types.Specifier
	if err := encoding.ReadObject(conn, &id, 16); err != nil {
		// log
		return
	}
	switch id {
	case modules.RPCSettings:
		h.rpcSettings(conn)
	case modules.RPCUpload:
		h.rpcUpload(conn)
	case modules.RPCRevise:
		h.rpcRevise(conn)
	case modules.RPCDownload:
		h.rpcDownload(conn)
	default:
		// log
	}
}

func (h *Host) rpcSettings(conn net.Conn) error {
	return encoding.WriteObject(conn, h.Settings())
}
