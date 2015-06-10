package types

// block.go defines the Block type for Sia, and provides some helper functions
// for working with blocks.

import (
	"bytes"

	"github.com/NebulousLabs/Sia/crypto"
)

type (
	// A Block is a summary of changes to the state that have occurred since the
	// previous block. Blocks reference the ID of the previous block (their
	// "parent"), creating the linked-list commonly known as the blockchain. Their
	// primary function is to bundle together transactions on the network. Blocks
	// are created by "miners," who collect transactions from other nodes, and
	// then try to pick a Nonce that results in a block whose BlockID is below a
	// given Target.
	Block struct {
		ParentID     BlockID
		Nonce        BlockNonce
		Timestamp    Timestamp
		MinerPayouts []SiacoinOutput
		Transactions []Transaction
	}

	// A BlockHeader, when encoded, is an 80-byte constant size field
	// containing enough information to do headers-first block downloading.
	// Hashing the header results in the block ID.
	BlockHeader struct {
		ParentID   BlockID
		Nonce      BlockNonce
		Timestamp  Timestamp
		MerkleRoot crypto.Hash
	}

	BlockHeight uint64
	BlockID     crypto.Hash
	BlockNonce  [8]byte
)

// CalculateCoinbase calculates the coinbase for a given height. The coinbase
// equation is:
//
//     coinbase := max(InitialCoinbase - height, MinimumCoinbase) * SiaCoinPrecision
func CalculateCoinbase(height BlockHeight) Currency {
	base := InitialCoinbase - uint64(height)
	if uint64(height) > InitialCoinbase || base < MinimumCoinbase {
		base = MinimumCoinbase
	}
	return NewCurrency64(base).Mul(SiaCoinPrecision)
}

// CalculateSubsidy takes a block and a height and determines the block
// subsidy.
func (b Block) CalculateSubsidy(height BlockHeight) Currency {
	subsidy := CalculateCoinbase(height)
	for _, txn := range b.Transactions {
		for _, fee := range txn.MinerFees {
			subsidy = subsidy.Add(fee)
		}
	}
	return subsidy
}

// CheckMinerPayouts compares the miner payouts to the subsidy and returns true
// if they are equal, false otherwise.
func (b Block) CheckMinerPayouts(height BlockHeight) bool {
	// Add up the payouts and check that all values are legal.
	var payoutSum Currency
	for _, payout := range b.MinerPayouts {
		if payout.Value.IsZero() {
			return false
		}
		payoutSum = payoutSum.Add(payout.Value)
	}

	// Compare the payouts to the subsidy.
	subsidy := b.CalculateSubsidy(height)
	if subsidy.Cmp(payoutSum) != 0 {
		return false
	}
	return true
}

// CheckTarget returns true if the block's ID meets the given target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// Header returns the header of a block.
func (b Block) Header() BlockHeader {
	return BlockHeader{
		ParentID:   b.ParentID,
		Nonce:      b.Nonce,
		Timestamp:  b.Timestamp,
		MerkleRoot: b.MerkleRoot(),
	}
}

// ID returns the ID of a Block, which is calculated by hashing the
// concatenation of the block's parent's ID, nonce, and the result of the
// b.MerkleRoot().
func (b Block) ID() BlockID {
	return BlockID(crypto.HashObject(b.Header()))
}

// MerkleRoot calculates the Merkle root of a Block. The leaves of the Merkle
// tree are composed of the Timestamp, the miner outputs (one leaf per
// payout), and the transactions (one leaf per transaction).
func (b Block) MerkleRoot() crypto.Hash {
	tree := crypto.NewTree()
	for _, payout := range b.MinerPayouts {
		tree.PushObject(payout)
	}
	for _, txn := range b.Transactions {
		tree.PushObject(txn)
	}
	return tree.Root()
}

// MinerPayoutID returns the ID of the miner payout at the given index, which
// is calculated by hashing the concatenation of the BlockID and the payout
// index.
func (b Block) MinerPayoutID(i int) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		b.ID(),
		i,
	))
}
