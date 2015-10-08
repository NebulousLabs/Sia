package miner

import (
	"time"
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
		if !m.miningOn {
			m.mining = false
			m.mu.Unlock(lockID)
			return
		}

		// Grab a block and try to solve it.
		bfw, target := m.blockForWork()
		m.increaseAttempts()
		m.mu.Unlock(lockID)
		b, solved := m.SolveBlock(bfw, target)
		if solved {
			err := m.SubmitBlock(b)
			if err != nil {
				m.log.Println("ERROR: An error occurred while cpu mining:", err)
			}
		}
	}
}

// CPUHashrate returns the cpu hashrate.
func (m *Miner) CPUHashrate() int {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	return int(m.hashRate)
}

// CPUMining indicates whether a cpu miner is running.
func (m *Miner) CPUMining() bool {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	return m.mining
}

// StartMining will spawn a thread to begin mining. The thread will only start
// mining if there is not another thread mining yet.
func (m *Miner) StartCPUMining() {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	m.miningOn = true
	go m.threadedMine()
}

// StopMining sets desiredThreads to 0, a value which is polled by mining
// threads. When set to 0, the mining threads will all cease mining.
func (m *Miner) StopCPUMining() {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)
	m.hashRate = 0
	m.miningOn = false
}
