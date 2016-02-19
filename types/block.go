package types

// block.go defines the Block type for Sia, and provides some helper functions
// for working with blocks.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

const (
	BlockHeaderSize = 80 // 32 (ParentID) + 8 (Nonce) + 8 (Timestamp) + 32 (MerkleRoot)
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
		ParentID     BlockID         `json:"parentid"`
		Nonce        BlockNonce      `json:"nonce"`
		Timestamp    Timestamp       `json:"timestamp"`
		MinerPayouts []SiacoinOutput `json:"minerpayouts"`
		Transactions []Transaction   `json:"transactions"`
	}

	// A BlockHeader, when encoded, is an 80-byte constant size field
	// containing enough information to do headers-first block downloading.
	// Hashing the header results in the block ID.
	BlockHeader struct {
		ParentID   BlockID     `json:"parentid"`
		Nonce      BlockNonce  `json:"nonce"`
		Timestamp  Timestamp   `json:"timestamp"`
		MerkleRoot crypto.Hash `json:"merkleroot"`
	}

	BlockHeight uint64
	BlockID     crypto.Hash
	BlockNonce  [8]byte
)

// CalculateCoinbase calculates the coinbase for a given height. The coinbase
// equation is:
//
//     coinbase := max(InitialCoinbase - height, MinimumCoinbase) * SiacoinPrecision
func CalculateCoinbase(height BlockHeight) Currency {
	base := InitialCoinbase - uint64(height)
	if uint64(height) > InitialCoinbase || base < MinimumCoinbase {
		base = MinimumCoinbase
	}
	return NewCurrency64(base).Mul(SiacoinPrecision)
}

// CalculateNumSiacoins calculates the number of siacoins in circulation at a
// given height.
func CalculateNumSiacoins(height BlockHeight) Currency {
	deflationBlocks := BlockHeight(InitialCoinbase - MinimumCoinbase)
	avgDeflationSiacoins := CalculateCoinbase(0).Add(CalculateCoinbase(height)).Div(NewCurrency64(2))
	if height <= deflationBlocks {
		deflationSiacoins := avgDeflationSiacoins.Mul(NewCurrency64(uint64(height + 1)))
		return deflationSiacoins
	}
	deflationSiacoins := avgDeflationSiacoins.Mul(NewCurrency64(uint64(deflationBlocks + 1)))
	trailingSiacoins := NewCurrency64(uint64(height - deflationBlocks)).Mul(CalculateCoinbase(height))
	return deflationSiacoins.Add(trailingSiacoins)
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

// ID returns the ID of a Block, which is calculated by hashing the
// concatenation of the block's parent's ID, nonce, and the result of the
// b.MerkleRoot().
func (h BlockHeader) ID() BlockID {
	return BlockID(crypto.HashObject(h))
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
func (b Block) MinerPayoutID(i uint64) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		b.ID(),
		i,
	))
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (b Block) MarshalSia(w io.Writer) error {
	w.Write(b.ParentID[:])
	w.Write(b.Nonce[:])
	w.Write(encoding.EncUint64(uint64(b.Timestamp)))
	return encoding.NewEncoder(w).EncodeAll(b.MinerPayouts, b.Transactions)
}

// UnmarshalSia implements the encoding.SiaUnmarshaler interface.
func (b *Block) UnmarshalSia(r io.Reader) error {
	io.ReadFull(r, b.ParentID[:])
	io.ReadFull(r, b.Nonce[:])
	tsBytes := make([]byte, 8)
	io.ReadFull(r, tsBytes)
	b.Timestamp = Timestamp(encoding.DecUint64(tsBytes))
	return encoding.NewDecoder(r).DecodeAll(&b.MinerPayouts, &b.Transactions)
}

// MarshalJSON marshales a block id as a hex string.
func (bid BlockID) MarshalJSON() ([]byte, error) {
	return json.Marshal(bid.String())
}

// String prints the block id in hex.
func (bid BlockID) String() string {
	return fmt.Sprintf("%x", bid[:])
}

// UnmarshalJSON decodes the json hex string of the block id.
func (bid *BlockID) UnmarshalJSON(b []byte) error {
	if len(b) != crypto.HashSize*2+2 {
		return crypto.ErrHashWrongLen
	}

	var bidBytes []byte
	_, err := fmt.Sscanf(string(b[1:len(b)-1]), "%x", &bidBytes)
	if err != nil {
		return errors.New("could not unmarshal BlockID: " + err.Error())
	}
	copy(bid[:], bidBytes)
	return nil
}
