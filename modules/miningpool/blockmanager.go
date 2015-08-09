package miningpool

import (
	"fmt"
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// SubmitBlockShare does TODO
func (mp *MiningPool) rpcSubmit(conn net.Conn) error {
	fmt.Println("Block attempt submitted") // testing statement
	// Read the Block in from conn
	var b types.Block
	err := encoding.ReadObject(conn, &b, 10e3) // TODO: figure out how big the read number should be
	if err != nil {
		return err
	}

	// TODO: Verify it beats target * targetMultiplier

	// TODO: Verify the block's payouts

	// Send a payment to the miner
	// TODO: Actually calculate how much a share should be
	share := b.MinerPayouts[1].Value
	updatedTxn, err := mp.sendPayment(share, b.MinerPayouts[0].UnlockHash)

	err = encoding.WriteObject(conn, updatedTxn)
	if err != nil {
		return err
	}

	// If the block beats target, submit it to the network

	return nil
}
