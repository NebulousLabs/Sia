package miner

import (
	"errors"
	"math/big"
	"net"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Negotiates a new payment channel with the pool
func (m *Miner) negotiatePaymentChannel() error {

	// Connect to the pool
	conn, err := net.DialTimeout("tcp", m.poolIP, 10e9)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = encoding.WriteObject(conn, types.Specifier{'c', 'h', 'a', 'n', 'n', 'e', 'l'})
	if err != nil {
		return err
	}

	// Generate and send a public key to the pool
	sk, pk, err := crypto.GenerateSignatureKeys()
	err = encoding.WriteObject(conn, pk)
	if err != nil {
		return err
	}

	// Receive the channel-funding transaction from the pool
	var channelTxn types.Transaction
	err = encoding.ReadObject(conn, &channelTxn, 10e3) // TODO: change to txn size
	if err != nil {
		return err
	}

	// Receive the pool's refund transaction
	var refundTxn types.Transaction
	err = encoding.ReadObject(conn, &refundTxn, 10e3) // TODO: change to txn size
	if err != nil {
		return err
	}

	// TODO: Check that the pool's transactions are correct (not trying to cheat us)

	// Sign the refund transaction, but with a timelock. This way the pool will
	// know it can get its money back if the miner disappears or fails to mine
	// any blocks.
	refundTxn.TransactionSignatures = append(refundTxn.TransactionSignatures, types.TransactionSignature{
		ParentID:       crypto.Hash(refundTxn.SiacoinInputs[0].ParentID),
		PublicKeyIndex: 1,
		Timelock:       m.height + 1008, // 1 week
		CoveredFields:  types.CoveredFields{WholeTransaction: true},
	})
	sigHash := refundTxn.SigHash(1)
	cryptoSig, err := crypto.SignHash(sigHash, sk)
	if err != nil {
		return err
	}
	refundTxn.TransactionSignatures[1].Signature = cryptoSig[:]

	// Send the refundTxn back to the pool
	err = encoding.WriteObject(conn, refundTxn)
	if err != nil {
		return err
	}

	// TODO: Verify the channelTxn has been signed and broadcasted

	// Send the pool an address so it can pay us
	err = encoding.WriteObject(conn, m.address)
	if err != nil {
		return err
	}

	m.poolSK = sk
	return nil
}

// Closes the specified payment channel by broadcasting the final transaction
// to the network. The miner recieves its payouts, but prevents more money from
// being sent through this channel
func (m *Miner) closeChannel() error {
	// TODO: First, make sure there is a channel to close?
	// Sign and broadcast the channel's output transaction
	sigHash := m.poolTransaction.SigHash(1)
	cryptoSig, err := crypto.SignHash(sigHash, m.poolSK)
	if err != nil {
		return err
	}
	m.poolTransaction.TransactionSignatures[1].Signature = cryptoSig[:]
	err = m.tpool.AcceptTransactionSet([]types.Transaction{m.poolTransaction})
	if err != nil {
		return err
	}

	// TODO: Tell pool that we closed the channel
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
func (m *Miner) PoolConnect(ip string) error {
	conn, err := net.DialTimeout("tcp", ip, 10e9)
	if err != nil {
		return err
	}
	defer conn.Close()
	m.poolIP = ip

	err = encoding.WriteObject(conn, types.Specifier{'s', 'e', 't', 't', 'i', 'n', 'g', 's'})
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
func (m *Miner) PoolHeaderForWork() (types.BlockHeader, types.Target, error) {
	// TODO: make sure we connected to a pool already

	// Get a header from the block manager
	header, target, err := m.HeaderForWork()
	if err != nil {
		return types.BlockHeader{}, types.Target{}, err
	}

	// TODO: Set the target to be easier
	target = target.MulDifficulty(big.NewRat(int64(m.targetMultiple), 1))

	// Change the payouts of the block manager's block
	block, err := m.reconstructBlock(header)
	if err != nil {
		return types.BlockHeader{}, types.Target{}, err
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

	// Generate the new pool-ready header and store a mapping to the non-pool
	// header it was derived from
	poolHeader := newBlock.Header()
	m.poolHeaderMem[poolHeader] = header

	return poolHeader, target, nil
}

// SubmitPoolHeader takes a header that has been solved and submits it
// to the pool
func (m *Miner) PoolSubmitHeader(bh types.BlockHeader) error {
	// TODO: make sure we connected to a pool already

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

	err = encoding.WriteObject(conn, types.Specifier{'s', 'u', 'b', 'm', 'i', 't'})
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, b)
	if err != nil {
		return err
	}

	// If the block beats the full target, submit it to the network also
	if b.CheckTarget(m.target) {
		err = m.SubmitBlock(b)
		err = nil
	}

	// Get the updated transaction (with the additional pay)
	var newTxn types.Transaction
	err = encoding.ReadObject(conn, &newTxn, 256)
	if err != nil {
		return err
	}
	// TODO: Check the txn for correctness
	m.poolTransaction = newTxn

	// For now, broadcast the payment channel to the network immediately
	// Let the pool know we're closing the channel so it can free some memory
	// TODO: Wait like 6 days until broadcasting/closing the payment channel
	err = m.closeChannel()
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
