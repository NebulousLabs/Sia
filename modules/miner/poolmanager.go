package miner

import (
	"fmt"
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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
	err = encoding.WriteObject(conn, [8]byte{'S', 'e', 't', 't', 'i', 'n', 'g', 's'})
	if err != nil {
		return err
	}

	var mps modules.MiningPoolSettings
	err = encoding.ReadObject(conn, &mps, 256)
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
	// For now, just get a normal block. We'll worry about making a
	// pool-specific block later on.
	return m.HeaderForWork()
}

// SubmitPoolHeader takes a header that has been solved and submits it
// to the pool
func (m *Miner) SubmitPoolHeader(bh types.BlockHeader) error {
	fmt.Println("pool header submit")
	// This function calls SubmitHeaderToPool reomtely (via RFC) in order to
	// submit the header to the pool and get credit for it
	return nil
}
