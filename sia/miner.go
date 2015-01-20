package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

// StartMining calls StartMining on the miner.
func (c *Core) StartMining() error {
	return c.miner.StartMining()
}

// StopMining calls StopMining on the miner.
func (c *Core) StopMining() error {
	return c.miner.StopMining()
}

// MinerInfo calls Info on the miner.
func (c *Core) MinerInfo() (components.MinerInfo, error) {
	return c.miner.Info()
}
