package siad

import (
	"github.com/NebulousLabs/Andromeda/network"
)

func (e *Environment) AddressBook() []network.NetAddress {
	return e.server.AddressBook()
}

func (e *Environment) RandomPeer() network.NetAddress {
	return e.server.RandomPeer()
}

func (e *Environment) NetAddress() network.NetAddress {
	return e.server.NetAddress()
}
