package miningpool

import (
	"net"

	"github.com/NebulousLabs/Sia/encoding"
)

type rpcID [8]byte

var (
	idSubmit   = rpcID{'S', 'u', 'b', 'm', 'i', 't'}
	idSettings = rpcID{'S', 'e', 't', 't', 'i', 'n', 'g', 's'}
)

func (mp *MiningPool) listen() {
	for {
		conn, err := mp.listener.Accept()
		if err != nil {
			return
		}
		go mp.handleConn(conn)
	}
}

func (mp *MiningPool) handleConn(conn net.Conn) {
	defer conn.Close()
	var id rpcID
	if err := encoding.ReadObject(conn, &id, 8); err != nil {
		// log
		return
	}
	switch id {
	case idSettings:
		mp.rpcSettings(conn)
	case idSubmit:
		mp.rpcSubmit(conn)
	default:
		// log
	}
}

func (mp *MiningPool) rpcSettings(conn net.Conn) error {
	return encoding.WriteObject(conn, mp.Settings())
}
