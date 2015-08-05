package miningpool

import (
	"fmt"
	"net"

	"github.com/NebulousLabs/Sia/types"
)

// SubmitBlockShare does TODO
func (mp *MiningPool) rpcSubmit(conn net.Conn) error {
	fmt.Println("Block attempt submitted") // testing statement
	return nil
}

// submitBlock submits a valid block to the network. It does not verify
// anything about the block before submitting (e.g. that the block has the
// proper payout to the pool)
func (mp *MiningPool) submitBlock(block types.Block) error {
	return nil
}
