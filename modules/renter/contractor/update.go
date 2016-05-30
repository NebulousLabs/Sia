package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// managedRenewContracts renews any contracts that are up for renewal, using
// the current allowance.
func (c *Contractor) managedRenewContracts() {
	c.mu.RLock()
	// Renew contracts when they enter the renew window.
	var renewSet []modules.RenterContract
	for _, contract := range c.contracts {
		if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight() {
			renewSet = append(renewSet, contract)
		}
	}
	endHeight := c.blockHeight + c.allowance.Period

	numSectors, err := maxSectors(c.allowance, c.hdb)
	c.mu.RUnlock()
	if err != nil {
		c.log.Println("WARN: could not calculate number of sectors allowance can support:", err)
		return
	}

	if len(renewSet) == 0 {
		// nothing to do
		return
	} else if numSectors == 0 {
		c.log.Printf("WARN: want to renew %v contracts, but allowance is too small", len(renewSet))
		return
	}

	filesize := numSectors * modules.SectorSize
	for _, contract := range renewSet {
		_, err := c.managedRenew(contract, filesize, endHeight)
		if err != nil {
			c.log.Println("WARN: failed to renew contract", contract.ID)
		}
	}
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change in the blockchain. Updates will always be called in order.
func (c *Contractor) ProcessConsensusChange(cc modules.ConsensusChange) {
	c.mu.Lock()
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
		if c.blockHeight > contract.EndHeight() {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		delete(c.contracts, id)
		c.log.Debugln("INFO: deleted expired contract", id)
	}

	c.lastChange = cc.ID
	err := c.save()
	if err != nil {
		c.log.Println(err)
	}
	c.mu.Unlock()

	// only attempt renewal if we are synced
	// (harmless otherwise, since hosts will reject our renewal attempts, but very slow)
	if c.cs.Synced() {
		c.managedRenewContracts()
	}
}
