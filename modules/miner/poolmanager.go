package miner

import (
	"fmt"
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Negotiates a new payment channel with the pool
func (m *Miner) negotiatePaymentChannel() error {
	return nil
}

// Closes the specified payment channel by broadcasting the final transaction
// to the network. The miner recieves its payouts, but prevents more money from
// being sent through this channel
func (m *Miner) closeChannel(poolTxn types.Transaction) error {
	return nil
}

// Connects to the pool hosted at the given ip. The miner negotiates a payment
// channel and gets certain values from the pool, like the payout address(es)
// and payout ratios (what percent goes to who)
func (m *Miner) ConnectToPool(ip string) error {
	fmt.Println("connect to pool: ", ip)
	conn, err := net.DialTimeout("tcp", ip, 10e9)
	if err != nil {
		return err
	}
	defer conn.Close()
	m.poolNetAddr = modules.NetAddress(ip)

	err = encoding.WriteObject(conn, [8]byte{'S', 'e', 't', 't', 'i', 'n', 'g', 's'})
	if err != nil {
		return err
	}

	var mps modules.MiningPoolSettings
	err = encoding.ReadObject(conn, &mps, 256)
	if err != nil {
		return err
	}
	fmt.Println(mps)
	//TODO: Save pool settings

	// Negotiate a payment channel with the pool
	err = m.negotiatePaymentChannel()
	if err != nil {
		return err
	}
	return nil
}

// PoolHeaderForWork returns the header of a block that is ready for pool
// mining. The block contains all the correct pool payouts. The header is
// meant to be grinded by a miner and, shuold the target be beat, resubmitted
// through SubmitHeaderToPool. Note that the target returned is a fraction of
// the real block target.
func (m *Miner) PoolHeaderForWork() (types.BlockHeader, types.Target) {
	fmt.Println("pool header get")
	// Get a header from the block manager

	// Change the payouts of the block manager's block

	// Generate the new pool-ready header and store a mapping to the non-pool
	// header it was derived from

	return types.BlockHeader{}, types.Target{}
}

// SubmitPoolHeader takes a header that has been solved and submits it
// to the pool
func (m *Miner) SubmitPoolHeader(bh types.BlockHeader) error {
	fmt.Println("pool header submit")
	// Reassamble the block that generated bh

	// Submit the block to the pool

	// For now, broadcast the payment channel to the network
	// TODO: Wait like 6 days until broadcasting/closing the payment channel
	err := m.closeChannel(m.poolTransaction)
	if err != nil {
		return err
	}

	// Negotiate a new payment channel since we closed the old one
	m.negotiatePaymentChannel()
	if err != nil {
		return err
	}

	return nil
}
