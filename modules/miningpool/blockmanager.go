package miningpool

import (
	"errors"
	"math/big"
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// rpcSubmit receives a block from the miner, checks it for correctness and
// then pays the miner accordingly via the already existing payment channel
func (mp *MiningPool) rpcSubmit(conn net.Conn) error {
	// Read the Block in from conn
	var b types.Block
	err := encoding.ReadObject(conn, &b, types.BlockSizeLimit) // TODO: figure out how big the read number should be
	if err != nil {
		return err
	}

	if !b.CheckTarget(mp.target.MulDifficulty(big.NewRat(int64(mp.MiningPoolSettings.TargetMultiple), 1))) {
		// TODO: log
		return errors.New("Block does not meet pool's target")
	}

	// Verify the block pays out to the pool
	poolPayout := b.MinerPayouts[1]
	pc, exists := mp.channels[poolPayout.UnlockHash]
	if !exists {
		return errors.New("Block does not payout to the pool")
	}

	// Verify the block pays at least the correct amount to the pool
	var totalSubsidy types.Currency
	for _, payout := range b.MinerPayouts {
		totalSubsidy.Add(payout.Value)
	}
	if poolPayout.Value.Cmp(totalSubsidy.MulRat(big.NewRat(int64(100-mp.MiningPoolSettings.MinerPercentCut), 100))) < 0 {
		return errors.New("Block does not pay enough to pool")
	}

	// TODO: Make sure the block is current (miners shouldn't be on forks)

	// TODO: Verify the block hasn't been submitted before
	_, exists = mp.spentHeaders[b.Header()]
	if exists {
		return errors.New("This block has been submitted already")
	}
	mp.spentHeaders[b.Header()] = struct{}{}

	// TODO: If the block beats the full target, submit it to the network
	if b.CheckTarget(mp.target) {
		err = mp.cs.AcceptBlock(b)
		if err != nil {
			return err
		}
	}

	// Send a payment to the miner (pool's subsidy * ((100 - poolPercentCut) / 100) / targetMultiplier
	share := b.MinerPayouts[1].Value // start with the amount the pool is getting paid this block
	share = share.Div(types.NewCurrency64(100)).Mul(types.NewCurrency64(100 - uint64(mp.MiningPoolSettings.PoolPercentCut)))
	share = share.Div(types.NewCurrency64(uint64(mp.MiningPoolSettings.TargetMultiple)))

	updatedTxn, err := mp.sendPayment(share, pc)
	err = encoding.WriteObject(conn, updatedTxn)
	if err != nil {
		return err
	}

	return nil
}
