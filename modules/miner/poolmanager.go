package miner

import (
	"errors"
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

// Reconstruct a block using its pool header by getting the block from the
// block manager, then changing the payouts.
func (m *Miner) reconstructPoolBlock(ph types.BlockHeader) (types.Block, error) {
	var zeroNonce [8]byte
	poolHeader := ph
	poolHeader.Nonce = zeroNonce

	// Convert poolHeader to a header that the block manager knows about
	lookupBH, exists := m.poolHeaderMem[poolHeader]
	if !exists {
		return types.Block{}, errors.New("Header is not a valid poolheader or is too old")
	}

	block, err := m.reconstructBlock(lookupBH)
	if err != nil {
		return types.Block{}, err
	}

	subsidy := block.MinerPayouts[0].Value
	minerPayout := subsidy.Div(types.NewCurrency64(100)).Mul(types.NewCurrency64(uint64(m.minerPercentCut)))
	poolPayout := subsidy.Sub(minerPayout)
	blockPayouts := []types.SiacoinOutput{
		types.SiacoinOutput{Value: minerPayout, UnlockHash: m.address},
		types.SiacoinOutput{Value: poolPayout, UnlockHash: m.poolPayoutAddress}}

	newBlock := types.Block{
		ParentID:     block.ParentID,
		Timestamp:    block.Timestamp,
		MinerPayouts: blockPayouts,
		Transactions: block.Transactions,
	}

	return newBlock, nil
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
	m.poolIP = ip

	err = encoding.WriteObject(conn, [8]byte{'S', 'e', 't', 't', 'i', 'n', 'g', 's'})
	if err != nil {
		return err
	}

	var mps modules.MiningPoolSettings
	err = encoding.ReadObject(conn, &mps, 256)
	if err != nil {
		return err
	}
	m.minerPercentCut = mps.MinerPercentCut
	m.targetMultiple = mps.TargetMultiple
	m.poolPayoutAddress = mps.Address
	fmt.Println("Miner payout: ", m.minerPercentCut)
	fmt.Println("Target multiple: ", m.targetMultiple)
	fmt.Println("Pool address: ", m.poolPayoutAddress)

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
	// TODO: make sure we connected to a pool already

	fmt.Println("pool header get")
	// Get a header from the block manager
	header, target := m.HeaderForWork()

	// TODO: Set the target to be easier
	//target = types.Target{uint32(target) * m.targetMultiple}

	// Change the payouts of the block manager's block
	block, err := m.reconstructBlock(header)
	if err != nil {
		return types.BlockHeader{}, types.Target{}
	}
	subsidy := block.MinerPayouts[0].Value
	fmt.Println(subsidy, target)
	minerPayout := subsidy.Div(types.NewCurrency64(100)).Mul(types.NewCurrency64(uint64(m.minerPercentCut)))
	poolPayout := subsidy.Sub(minerPayout)
	blockPayouts := []types.SiacoinOutput{
		types.SiacoinOutput{Value: minerPayout, UnlockHash: m.address},
		types.SiacoinOutput{Value: poolPayout, UnlockHash: m.poolPayoutAddress}}

	fmt.Println(minerPayout)
	fmt.Println(poolPayout)

	newBlock := types.Block{
		ParentID:     block.ParentID,
		Timestamp:    block.Timestamp,
		MinerPayouts: blockPayouts,
		Transactions: block.Transactions,
	}

	// Generate the new pool-ready header and store a mapping to the non-pool
	// header it was derived from
	poolHeader := newBlock.Header()
	m.poolHeaderMem[poolHeader] = header

	return poolHeader, target
}

// SubmitPoolHeader takes a header that has been solved and submits it
// to the pool
func (m *Miner) SubmitPoolHeader(bh types.BlockHeader) error {
	// TODO: make sure we connected to a pool already

	fmt.Println("pool header submit")
	// Reassamble the block that generated bh
	lockID := m.mu.Lock()
	b, err := m.reconstructPoolBlock(bh)
	m.mu.Unlock(lockID)
	if err != nil {
		return err
	}

	// Submit the block to the pool
	conn, err := net.DialTimeout("tcp", m.poolIP, 10e9)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = encoding.WriteObject(conn, [8]byte{'S', 'u', 'b', 'm', 'i', 't'})
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, b)
	if err != nil {
		return err
	}

	// Get the updated transaction (with the additional pay)
	var newTxn types.Transaction
	err = encoding.ReadObject(conn, &newTxn, 256)
	if err != nil {
		return err
	}
	// TODO: Check the txn for correctness
	m.poolTransaction = newTxn

	// For now, broadcast the payment channel to the network
	// TODO: Wait like 6 days until broadcasting/closing the payment channel
	err = m.closeChannel(m.poolTransaction)
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
