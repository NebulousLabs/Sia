// Package siatest contains a bunch of testing utilities and helper functions.
// Siatest is able to spin up full nodes, groups of full nodes, and then
// provides helper functions for interacting with the full nodes, such as
// mining a block or getting started with renting + hosting.
package siatest

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// serverTester contains a server and a set of channels for keeping all of the
// modules synchronized during testing.
type TestNode struct {
	// The modules of the node. Modules that are not initialized will be nil.
	ConsensusSet    modules.ConsensusSet
	Explorer        modules.Explorer
	Gateway         modules.Gateway
	Host            modules.Host
	Miner           modules.TestMiner
	Renter          modules.Renter
	TransactionPool modules.TransactionPool
	Wallet          modules.Wallet

	// The key of the wallet, which is used to re-unlock the wallet when the
	// node resets.
	WalletKey crypto.TwofishKey
}

// NewTestNode will create a new test node. The inputs to the function are the
// respective 'New' calls for each module. We need to use this awkward method
// of initialization because the siatest package cannot import any of the
// modules directly (so that the modules may use the siatest package to test
// themselves).
func NewTestNode(dir string, newGateway func(string, bool, string) (modules.Gateway, error)) { // , newConsensusSet func(), newTransactionPool func(), newExplorer func(), newWallet func(), newRenter func(), newHost func()) (*TestNode, error) {
	_, err := newGateway("localhost:0", false, filepath.Join(dir, "gateway"))
	if err != nil {
		println("error!")
		return
	}
	println("it worked")
}
