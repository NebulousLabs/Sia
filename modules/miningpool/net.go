package miningpool

import (
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

var (
	idChannel  = types.Specifier{'c', 'h', 'a', 'n', 'n', 'e', 'l'}
	idSettings = types.Specifier{'s', 'e', 't', 't', 'i', 'n', 'g', 's'}
	idSubmit   = types.Specifier{'s', 'u', 'b', 'm', 'i', 't'}
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
	var id types.Specifier
	if err := encoding.ReadObject(conn, &id, types.SpecifierLen); err != nil {
		// log
		return
	}
	switch id {
	case idChannel:
		mp.rpcNegotiatePaymentChannel(conn)
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
