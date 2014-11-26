package siad

import (
	"github.com/NebulousLabs/Andromeda/network"
)

func (e *Environment) AddressBook() []network.NetAddress {
	return e.server.AddressBook()
}
