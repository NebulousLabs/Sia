package miner

import (
	"time"
)

// threadedMine starts a gothread that does CPU mining. threadedMine is the
// only function that should be setting the mining flag to true.
func (m *Miner) threadedMine() {
	// There should not be another thread mining, and mining should be enabled.
	lockID := m.mu.Lock()
	if m.mining || !m.miningOn {
		m.mu.Unlock(lockID)
		return
	}
	m.mining = true
	m.mu.Unlock(lockID)

	// Solve blocks repeatedly.
	for {
		// Kill the thread if mining has been turned off.
		lockID := m.mu.Lock()
		m.cycleStart = time.Now()
		if !m.miningOn {
			m.mining = false
			m.mu.Unlock(lockID)
			return
		}

		// Grab a block and try to solve it.
		bfw, target := m.blockForWork()
		m.mu.Unlock(lockID)
		b, solved := m.SolveBlock(bfw, target)
		if solved {
			err := m.SubmitBlock(b)
			if err != nil {
				m.log.Println("ERROR: An error occurred while cpu mining:", err)
			}
		}

		// Update the hashrate. If the block was solved, the full set of
		// iterations was not completed, so the hashrate should not be updated.
		lockID = m.mu.Lock()
		if !solved {
			nanosecondsElapsed := 1 + time.Since(m.cycleStart).Nanoseconds() // Add 1 to prevent divide by zero errors.
			m.hashRate = 1e9 * iterationsPerAttempt / nanosecondsElapsed
		}
		m.mu.Unlock(lockID)
	}
}

// CPUHashrate returns an estimated cpu hashrate.
func (m *Miner) CPUHashrate() int {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	return int(m.hashRate)
}

// CPUMining indicates whether the cpu miner is running.
func (m *Miner) CPUMining() bool {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	return m.mining
}

// StartCPUMining will start a single threaded cpu miner. If the miner is
// already running, nothing will happen.
func (m *Miner) StartCPUMining() {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	m.miningOn = true
	go m.threadedMine()
}

// StopCPUMining will stop the cpu miner. If the cpu miner is already stopped,
// nothing will happen.
func (m *Miner) StopCPUMining() {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	m.hashRate = 0
	m.miningOn = false
}
