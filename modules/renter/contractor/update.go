package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusChange will be called by the consensus set every time there
// is a change in the blockchain. Updates will always be called in order.
func (c *Contractor) ProcessConsensusChange(cc modules.ConsensusChange) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, block := range cc.RevertedBlocks {
		if block.ID() != types.GenesisID {
			c.blockHeight--
		}
	}
	for _, block := range cc.AppliedBlocks {
		if block.ID() != types.GenesisID {
			c.blockHeight++
		}
	}

	// delete expired contracts
	var expired []types.FileContractID
	for id, contract := range c.contracts {
		// TODO: offset this by some sort of confirmation height?
		if c.blockHeight > contract.LastRevision.NewWindowStart {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		delete(c.contracts, id)
		c.log.Debugln("INFO: deleted expired contract", id)
	}

	// renew contracts
	// TODO: re-enable this functionality
	// if c.blockHeight+c.allowance.RenewWindow >= c.renewHeight {
	// 	c.renewHeight += c.allowance.Period
	// 	go c.threadedRenewContracts(c.allowance, c.renewHeight)
	// 	// reset the spentPeriod metric
	// 	c.spentPeriod = types.ZeroCurrency
	// }

	c.lastChange = cc.ID
	err := c.save()
	if err != nil {
		c.log.Println(err)
	}
}
