package miningpool

import (
	"fmt"
	"net"

	//"github.com/NebulousLabs/Sia/types"
)

// SubmitBlockShare does TODO
func (mp *MiningPool) rpcSubmit(conn net.Conn) error {
	fmt.Println("Block attempt submitted") // testing statement
	// Read the Block in from conn

	// Verify it beats target * targetMultiplier

	// TODO: Verify the block's payouts

	// Send a payment to the miner

	// If the block beats target, submit it to the network

	return nil
}
