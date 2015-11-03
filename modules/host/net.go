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
		return
	}
	var err error
	switch id {
	case modules.RPCSettings:
		err = h.rpcSettings(conn)
	case modules.RPCUpload:
		err = h.rpcUpload(conn)
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

func (h *Host) rpcSettings(conn net.Conn) error {
	return encoding.WriteObject(conn, h.Settings())
}
