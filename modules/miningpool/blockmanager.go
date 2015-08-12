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
	err := encoding.ReadObject(conn, &b, 1e6) // TODO: figure out how big the read number should be
	if err != nil {
		return err
	}

	// TODO: Verify it beats target * targetMultiplier

	// TODO: Verify the block's payouts

	// TODO: Verify the block hasn't been submitted before

	// TODO: If the block beats the full target, submit it to the network

	// Send a payment to the miner (pool's subsidy * ((100 - poolPercentCut) / 100) / targetMultiplier
	share := b.MinerPayouts[1].Value // start with the amount the pool is getting paid this block
	share = share.Div(types.NewCurrency64(100)).Mul(types.NewCurrency64(100 - uint64(mp.MiningPoolSettings.PoolPercentCut)))
	fmt.Println(share)
	share = share.Div(types.NewCurrency64(uint64(mp.MiningPoolSettings.TargetMultiple)))
	fmt.Println(share)
	fmt.Println("Pool sends 0.", share.Mul(types.NewCurrency64(1000)).Div(b.MinerPayouts[1].Value), "% of it's cut to the miner")

	updatedTxn, err := mp.sendPayment(share, b.MinerPayouts[0].UnlockHash)
	err = encoding.WriteObject(conn, updatedTxn)
	if err != nil {
		return err
	}

	return nil
}
