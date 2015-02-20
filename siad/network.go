package main

import (
	"fmt"
	"time"

	"github.com/NebulousLabs/Sia/network"
)

var (
	// hard-coded addresses used when bootstrapping
	BootstrapPeers = []network.Address{
		"23.239.14.98:9988",
	}
)

// bootstrap bootstraps to the network, downlading all of the blocks and
// establishing a peer list.
func (d *daemon) bootstrap() {
	// Establish an initial peer list.
	// TODO: add more bootstrap peers.
	err := d.gateway.Bootstrap(BootstrapPeers[0])
	if err != nil {
		fmt.Println(`Warning: no peers responded to bootstrap request.
Add peers manually to enable bootstrapping.`)
		// TODO: wait for new peers?
		return
	}

	// Every 2 minutes, call Synchronize. In theory this shouldn't be
	// necessary; it's here to improve robustness.
	for {
		go d.gateway.Synchronize()
		time.Sleep(time.Minute * 2)
	}
}
