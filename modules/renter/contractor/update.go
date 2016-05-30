package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// managedRenewContracts renews any contracts that are up for renewal, using
// the current allowance.
func (c *Contractor) managedRenewContracts() {
	c.mu.RLock()
	// Renew contracts when they enter the renew window. All contracts are
	// renewed to the same height, using available remaining funds.
	var renewSet []modules.RenterContract
	var currentSpent types.Currency
	for _, contract := range c.contracts {
		if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight() {
			renewSet = append(renewSet, contract)
		} else {
			// only count the contracts that aren't up for renewal
			currentSpent = currentSpent.Add(contract.FileContract.ValidProofOutputs[0].Value)
		}
	}
	endHeight := c.blockHeight + c.allowance.Period
	allowance := c.allowance
	c.mu.RUnlock()

	if len(renewSet) == 0 || allowance.Hosts == 0 {
		// nothing to do
		return
	}

	if len(renewSet) > 0 && allowance.Funds.Cmp(currentSpent) < 0 {
		c.log.Printf("WARN: want to renew %v contracts, but allowance is too small", len(renewSet))
		return
	}

	// calculate how many sectors we can support when renewing
	var hosts []modules.HostDBEntry
	for _, contract := range renewSet {
		h, ok := c.hdb.Host(contract.NetAddress)
		if !ok {
			c.log.Printf("WARN: want to renew contract with %v, but that host is no longer in the host DB", contract.NetAddress)
			continue
		}
		hosts = append(hosts, h)
	}
	if len(hosts) == 0 {
		return
	}
	funds := allowance.Funds.Sub(currentSpent)
	costPerSector := averageHostPrice(hosts).Mul64(uint64(len(hosts))).Mul64(modules.SectorSize).Mul64(uint64(allowance.Period))
	numSectors, err := funds.Div(costPerSector).Uint64()
	if err != nil {
		// if there was an overflow, something is definitely wrong
		c.log.Println("WARN: allowance resulted in unexpectedly large contract size")
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

	c.managedRenewContracts()
}
