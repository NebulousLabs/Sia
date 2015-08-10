package host

import (
	"net"

	"github.com/NebulousLabs/Sia/encoding"
)

type rpcID [8]byte

var (
	idSettings = rpcID{'S', 'e', 't', 't', 'i', 'n', 'g', 's'}
	idContract = rpcID{'C', 'o', 'n', 't', 'r', 'a', 'c', 't'}
	idDownload = rpcID{'D', 'o', 'w', 'n', 'l', 'o', 'a', 'd'}
	// deprecated
	idRetrieve = rpcID{'R', 'e', 't', 'r', 'i', 'e', 'v', 'e'}
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
	var id rpcID
	if err := encoding.ReadObject(conn, &id, 8); err != nil {
		// log
		return
	}
	switch id {
	case idSettings:
		h.rpcSettings(conn)
	case idContract:
		h.rpcContract(conn)
	case idDownload:
		h.rpcDownload(conn)
	// deprecated
	case idRetrieve:
		h.rpcRetrieve(conn)
	default:
		// log
	}
}

func (h *Host) rpcSettings(conn net.Conn) error {
	return encoding.WriteObject(conn, h.Settings())
}
