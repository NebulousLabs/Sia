package miner

// testminer.go implements the TestMiner interface, whose primary purpose is
// integration testing.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"unsafe"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// solveAttempts is the number of times that SolveBlock will try to solve a
	// block before giving up.
	solveAttempts = 16 * 1024
)

// BlockForWork returns a block that is ready for nonce grinding, along with
// the root hash of the block.
func (m *Miner) BlockForWork() (b types.Block, merkleRoot crypto.Hash, t types.Target, err error) {
	// Check if the wallet is unlocked. If the wallet is unlocked, make sure
	// that the miner has a recent address.
	if !m.wallet.Unlocked() {
		err = modules.ErrLockedWallet
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	err = m.checkAddress()
	if err != nil {
		return
	}

	b, t = m.blockForWork()
	merkleRoot = b.MerkleRoot()
	return b, merkleRoot, t, nil
}

// AddBlock adds a block to the consensus set.
func (m *Miner) AddBlock() (types.Block, error) {
	block, err := m.FindBlock()
	if err != nil {
		return types.Block{}, err
	}
	err = m.cs.AcceptBlock(block)
	if err != nil {
		return types.Block{}, err
	}
	return block, nil
}

// FindBlock finds at most one block that extends the current blockchain.
func (m *Miner) FindBlock() (types.Block, error) {
	m.mu.Lock()
	if !m.wallet.Unlocked() {
		return types.Block{}, modules.ErrLockedWallet
	}
	err := m.checkAddress()
	if err != nil {
		return types.Block{}, err
	}
	m.mu.Unlock()

	// Get a block for work.
	m.mu.Lock()
	bfw, target := m.blockForWork()
	m.mu.Unlock()

	block, ok := m.SolveBlock(bfw, target)
	if !ok {
		return types.Block{}, errors.New("could not solve block using limited hashing power")
	}
	return block, nil
}

// SolveBlock takes a block and a target and tries to solve the block for the
// target. A bool is returned indicating whether the block was successfully
// solved.
func (m *Miner) SolveBlock(b types.Block, target types.Target) (types.Block, bool) {
	// Assemble the header.
	merkleRoot := b.MerkleRoot()
	header := make([]byte, 80)
	copy(header, b.ParentID[:])
	binary.LittleEndian.PutUint64(header[40:48], uint64(b.Timestamp))
	copy(header[48:], merkleRoot[:])

	var nonce uint64
	for i := 0; i < solveAttempts; i++ {
		id := crypto.HashBytes(header)
		if bytes.Compare(target[:], id[:]) >= 0 {
			copy(b.Nonce[:], header[32:40])
			return b, true
		}
		*(*uint64)(unsafe.Pointer(&header[32])) = nonce
		nonce++
	}
	return b, false
}
