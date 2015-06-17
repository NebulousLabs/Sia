package miner

import (
	"bytes"
	"encoding/binary"
	"time"
	"unsafe"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// increaseAttempts is the miner's way of guaging it's own hashrate. After it's
// made 100 attempts to find a block, it calculates a hashrate based on how
// much time has passed. The number of attempts in progress is set to 0
// whenever mining starts or stops, which prevents weird low values from
// cropping up.
func (m *Miner) increaseAttempts() {
	m.attempts++
	if m.attempts >= 25 { // Waiting for 25 attempts minimizes hashrate variance.
		m.hashRate = int64((m.attempts * iterationsPerAttempt * 1e9)) / (time.Now().UnixNano() - m.startTime.UnixNano())
		m.startTime = time.Now()
		m.attempts = 0
	}
}

// threadedMine starts a gothread that does CPU mining. threadedMine is the
// only function that should be setting the mining flag to true.
func (m *Miner) threadedMine() {
	// There should not be another thread mining, and mining should be enabled.
	m.mu.Lock()
	if m.mining || !m.miningOn {
		m.mu.Unlock()
		return
	}
	m.mining = true
	m.mu.Unlock()

	// Solve blocks repeatedly.
	for {
		// Kill the thread if mining has been turned off.
		m.mu.Lock()
		if !m.miningOn {
			m.mining = false
			m.mu.Unlock()
			return
		}

		// Grab a block and try to solve it.
		bfw, target := m.blockForWork()
		m.increaseAttempts()
		m.mu.Unlock()
		b, solved := m.SolveBlock(bfw, target)

		// If solved, submit the block
		if solved {
			_ = m.SubmitBlock(b) // TODO: Log an error.
		}
	}
}

// CPUHashrate returns the cpu hashrate.
func (m *Miner) CPUHashrate() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int(m.hashRate)
}

// CPUMining indicates whether a cpu miner is running.
func (m *Miner) CPUMining() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mining
}

// FindBlock finds at most one block that extends the current blockchain and
// then submits that block to the consensus set.
//
// TODO: Block should not automatically be submitted to the consensus set.
func (m *Miner) FindBlock() (types.Block, bool, error) {
	m.mu.Lock()
	bfw, target := m.blockForWork()
	m.mu.Unlock()

	var err error
	block, solved := m.SolveBlock(bfw, target)
	if solved {
		err = m.SubmitBlock(block)
	}
	return block, solved, err
}

// SolveBlock takes a block, target, and number of iterations as input and
// tries to find a block that meets the target. This function can take a long
// time to complete, and should not be called with a lock.
func (m *Miner) SolveBlock(b types.Block, target types.Target) (types.Block, bool) {
	// Assemble the header.
	merkleRoot := b.MerkleRoot()
	header := make([]byte, 80)
	copy(header, b.ParentID[:])
	binary.LittleEndian.PutUint64(header[40:48], uint64(b.Timestamp))
	copy(header[48:], merkleRoot[:])

	nonce := (*uint64)(unsafe.Pointer(&header[32]))
	for i := 0; i < iterationsPerAttempt; i++ {
		id := crypto.HashBytes(header)
		if bytes.Compare(target[:], id[:]) >= 0 {
			copy(b.Nonce[:], header[32:40])
			return b, true
		}
		*nonce++
	}
	return b, false
}

// StartMining will spawn a thread to begin mining. The thread will only start
// mining if there is not another thread mining yet.
func (m *Miner) StartCPUMining() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.miningOn = true
	go m.threadedMine()
}

// StopMining sets desiredThreads to 0, a value which is polled by mining
// threads. When set to 0, the mining threads will all cease mining.
func (m *Miner) StopCPUMining() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hashRate = 0
	m.miningOn = false
}
