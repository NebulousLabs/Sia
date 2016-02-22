package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// renewThreshold calculates an appropriate renewThreshold for a given period.
func renewThreshold(period types.BlockHeight) types.BlockHeight {
	threshold := period / 4
	if threshold > 1000 {
		threshold = 1000
	}
	return threshold
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change in the blockchain. Updates will always be called in order.
func (c *Contractor) ProcessConsensusChange(cc modules.ConsensusChange) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.blockHeight != 0 || cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID() != types.GenesisBlock.ID() {
		c.blockHeight += types.BlockHeight(len(cc.AppliedBlocks))
		c.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	}

	// renew/replace contracts
	if c.blockHeight+renewThreshold(c.allowance.Period) > c.renewHeight {
		// set new renewHeight immediately; this prevents new renew/replace
		// goroutines from being spawned on every block until they succeed.
		// However, this means those goroutines must reset the renewHeight
		// manually if they fail.
		oldHeight := c.renewHeight
		c.renewHeight += c.allowance.Period

		// if newAllowance is set, use the new allowance to form new
		// contracts. Otherwise, renew existing contracts.
		if c.newAllowance.Hosts != 0 {
			go func() {
				err := c.formContracts(c.newAllowance)
				if err != nil {
					c.log.Println("WARN: failed to form contracts with new allowance:", err)
				}
				c.mu.Lock()
				defer c.mu.Unlock()
				if err == nil {
					// if contract formation succeeded, clear newAllowance
					c.allowance = c.newAllowance
					c.newAllowance = modules.Allowance{}
				} else {
					// otherwise, reset renewHeight so that we'll try again on the next block
					c.renewHeight = oldHeight
				}
			}()
		} else {
			go func() {
				newHeight := c.renewHeight + c.allowance.Period
				for _, contract := range c.contracts {
					if contract.FileContract.WindowStart == c.renewHeight {
						_, err := c.managedRenew(contract.ID, newHeight)
						if err != nil {
							c.log.Println("WARN: failed to renew contract", contract.ID, ":", err)
						}
					}
				}
				// TODO: reset renewHeight if too many rewewals failed.
			}()
		}
	}
}
