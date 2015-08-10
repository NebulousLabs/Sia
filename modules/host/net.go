package host

import (
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
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

func (h *Host) handleConn(conn net.Conn) {
	defer conn.Close()
	var id [8]byte
	if err := encoding.ReadObject(conn, &id, 8); err != nil {
		// log
		return
	}
	switch id {
	case modules.RPCSettings:
		h.rpcSettings(conn)
	case modules.RPCContract:
		h.rpcContract(conn)
	case modules.RPCDownload:
		h.rpcDownload(conn)
	// deprecated
	case modules.RPCRetrieve:
		h.rpcRetrieve(conn)
	default:
		// log
	}
}

func (h *Host) rpcSettings(conn net.Conn) error {
	return encoding.WriteObject(conn, h.Settings())
}
