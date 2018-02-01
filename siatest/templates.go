package siatest

import "github.com/NebulousLabs/Sia/node"

var (
	// MinerTemplate is a template for a Sia node that has a functioning
	// miner. The node has a miner and all dependencies, but no other
	// modules.
	MinerTemplate = node.NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            false,
		CreateMiner:           true,
		CreateRenter:          false,
		CreateTransactionPool: true,
		CreateWallet:          true,
	}
)

// Miner returns an MinerTemplate filled out with the provided dir.
func Miner(dir string) node.NodeParams {
	template := MinerTemplate
	template.Dir = dir
	return template
}
