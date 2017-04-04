package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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

	// archive expired contracts
	var expired []types.FileContractID
	for id, contract := range c.contracts {
		if c.blockHeight > contract.EndHeight() {
			// No need to wait for extra confirmations - any processes which
			// depend on this contract should have taken care of any issues
			// already.
			expired = append(expired, id)
			// move to oldContracts
			c.oldContracts[id] = contract
		}
	}
	// delete expired contracts (can't delete while iterating)
	for _, id := range expired {
		delete(c.contracts, id)
		c.log.Println("INFO: archived expired contract", id)
	}

	// If we have entered the next period, update currentPeriod
	// NOTE: "period" refers to the duration of contracts, whereas "cycle"
	// refers to how frequently the period metrics are reset.
	// TODO: How to make this more explicit.
	cycleLen := c.allowance.Period - c.allowance.RenewWindow
	if c.blockHeight > c.currentPeriod+cycleLen {
		c.currentPeriod += cycleLen
		// COMPATv1.0.4-lts
		// if we were storing a special metrics contract, it will be invalid
		// after we enter the next period.
		delete(c.oldContracts, metricsContractID)
	}

	c.lastChange = cc.ID
	err := c.save()
	if err != nil {
		c.log.Println("Unable to save while processing a consensus change:", err)
	}
	c.mu.Unlock()

	// Only attempt contract formation/renewal if we are synced
	// (harmless if not synced, since hosts will reject our renewal attempts,
	// but very slow).
	if cc.Synced {
		go func() {
			// Add the goroutine to the thread group.
			err := c.tg.Add()
			if err != nil {
				return
			}
			defer c.tg.Done()

			// Only one goroutine should be editing contracts at a time.
			if !c.editLock.TryLock() {
				return
			}
			defer c.editLock.Unlock()

			// Renew any (online) contracts that have entered the renew window.
			err = c.managedRenewContracts()
			if err != nil {
				c.log.Debugln("WARN: failed to renew contracts after processing a consensus chage:", err)
			}

			// If we don't have enough (online) contracts, form new ones.
			c.mu.RLock()
			a := c.allowance
			remaining := int(a.Hosts) - len(c.onlineContracts())
			c.mu.RUnlock()
			if remaining <= 0 {
				return
			}
			max, err := maxSectors(a, c.hdb, c.tpool)
			if err != nil {
				c.log.Debugln("ERROR: couldn't calculate maxSectors after processing a consensus change:", err)
				return
			}
			// Only allocate half as many sectors as the max. This leaves some leeway
			// for replacing contracts, transaction fees, etc.
			numSectors := max / 2
			err = c.managedFormAllowanceContracts(remaining, numSectors, a)
			if err != nil {
				c.log.Debugln("WARN: failed to form contracts after processing a consensus change:", err)
			}
		}()
	}
}
