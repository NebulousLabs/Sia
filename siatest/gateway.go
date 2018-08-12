package siatest

import (
	"github.com/NebulousLabs/Sia/build"
)

// hasPeer checks if peer is a peer of tn.
func (tn *TestNode) hasPeer(peer *TestNode) (bool, error) {
	ga := peer.Server.GatewayAddress()
	peerAddr := ga.Host() + ga.Port()
	gwg, err := tn.GatewayGet()
	if err != nil {
		return false, build.ExtendErr("failed to get gateway information", err)
	}
	for _, peer := range gwg.Peers {
		if peerAddr == peer.NetAddress.Host()+peer.NetAddress.Port() {
			return true, nil
		}
	}
	return false, nil
}
