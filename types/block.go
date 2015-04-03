package types

import (
	"bytes"

	"github.com/NebulousLabs/Sia/crypto"
)

type (
	BlockHeight uint64
	BlockID     crypto.Hash
)

// A Block is a summary of changes to the state that have occurred since the
// previous block. Blocks reference the ID of the previous block (their
// "parent"), creating the linked-list commonly known as the blockchain. Their
// primary function is to bundle together transactions on the network. Blocks
// are created by "miners," who collect transactions from other nodes, and
// then try to pick a Nonce that results in a block whose BlockID is below a
// given Target.
type Block struct {
	ParentID     BlockID
	Nonce        uint64
	Timestamp    Timestamp
	MinerPayouts []SiacoinOutput
	Transactions []Transaction
}

// CalculateCoinbase calculates the coinbase for a given height. The coinbase
// equation is:
//
//     coinbase := max(InitialCoinbase - height, MinimumCoinbase) * CoinbaseAugment
func CalculateCoinbase(height BlockHeight) (c Currency) {
	base := InitialCoinbase - uint64(height)
	if base < MinimumCoinbase {
		base = MinimumCoinbase
	}

	return NewCurrency64(base).Mul(NewCurrency(CoinbaseAugment))
}

// ID returns the ID of a Block, which is calculated by hashing the
// concatenation of the block's parent ID, nonce, and Merkle root.
func (b Block) ID() BlockID {
	return BlockID(crypto.HashAll(
		b.ParentID,
		b.Nonce,
		b.MerkleRoot(),
	))
}

// CheckTarget returns true if the block's ID meets the given target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// MerkleRoot calculates the Merkle root of a Block. The leaves of the Merkle
// tree are composed of the Timestamp, the miner outputs (one leaf per
// payout), and the transactions (one leaf per transaction).
func (b Block) MerkleRoot() crypto.Hash {
	tree := crypto.NewTree()
	tree.PushObject(b.Timestamp)
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
