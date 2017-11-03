package api

import (
	"encoding/json"
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"log"
)

//ProcessConsensusChange subscription allows the API to react in real time to consensus change events
func (api *API) ProcessConsensusChange(cc modules.ConsensusChange) {
	//We only output websocket events if we're synced
	if !cc.Synced {
		return
	}

	var ecc ExplorerConsensusChange
	for _, block := range cc.AppliedBlocks {
		_, height, exists := api.explorer.Block(block.ID())
		if exists {
			ecc.AppliedBlocks = append(ecc.AppliedBlocks, api.buildExplorerBlock(height, block))
		} else if build.DEBUG {
			log.Printf("Unable to broadcast block %s, it most likely wasn't found in the explorer db", block.ID())
		}
	}

	for _, block := range cc.RevertedBlocks {
		ecc.RevertedBlocks = append(ecc.RevertedBlocks, block.ID())
	}

	b, err := json.Marshal(ecc)
	if err != nil && build.DEBUG {
		log.Printf("Unable to marshal block. Error: %v", err)
		return
	}
	api.hub.broadcastBlock <- b
}

//ReceiveUpdatedUnconfirmedTransactions subscription allows the API to react in real time to received pending transactions
func (api *API) ReceiveUpdatedUnconfirmedTransactions(diff *modules.TransactionPoolDiff) {
	//We need the lock because we hold the some state in the API.
	api.mu.Lock()
	defer api.mu.Unlock()
	resp := ExplorerUnconfirmedTransactionChange{
		AppliedTransactions:  make([]ExplorerTransaction, 0),
		RevertedTransactions: make([]types.TransactionID, 0),
	}
	if len(diff.AppliedTransactions) == 0 && len(diff.RevertedTransactions) == 0 {
		return
	}
	for i := range diff.RevertedTransactions {
		txids := api.unconfirmedSets[diff.RevertedTransactions[i]]
		for i := range txids {
			resp.RevertedTransactions = append(resp.RevertedTransactions, txids[i])
		}
		delete(api.unconfirmedSets, diff.RevertedTransactions[i])
	}
	for _, unconfirmedTxnSet := range diff.AppliedTransactions {
		api.unconfirmedSets[unconfirmedTxnSet.ID] = unconfirmedTxnSet.IDs
		for _, tx := range unconfirmedTxnSet.Transactions {
			resp.AppliedTransactions = append(resp.AppliedTransactions, api.buildExplorerTransaction(0, types.GenesisID, tx))
		}
	}
	b, err := json.Marshal(resp)
	if err != nil && build.DEBUG {
		log.Printf("Unable to marshal transaction set. Error: %v", err)
		return
	}
	api.hub.broadcastTx <- b
}
