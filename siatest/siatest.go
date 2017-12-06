// Package siatest contains a bunch of testing utilities and helper functions.
// Siatest is able to spin up full nodes, groups of full nodes, and then
// provides helper functions for interacting with the full nodes, such as
// mining a block or getting started with renting + hosting.
package siatest

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/errors"
)

// NewTestNodeParams contains a bunch of parameters for creating a new test
// node.
//
// Each module is instantiated separately. There are several ways to
// instantiate a module, not all available for each module:
//		+ Pass the module in directly (everything else should be nil)
//		+ Pass the newFunc (everything else should be nil)
//		+ Pass the newFuncDeps and deps in (everything else shoudl be nil)
//		+ Pass 'nil' in for everything (module will not be instantiated)
type NewTestNodeParams struct {
	Dir string

	// Gateway.
	CreateGateway bool
	Gateway       modules.Gateway
}

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
func NewTestNode(params NewTestNodeParams) (*TestNode, error) {
	dir := params.Dir

	g, err := func() (modules.Gateway, error) {
		if params.CreateGateway && params.Gateway != nil {
			return nil, errors.New("cannot both create a gateway and use a passed in gateway")
		}
		if params.Gateway != nil {
			return params.Gateway, nil
		}
		if params.CreateGateway {
			return gateway.New("localhost:0", false, filepath.Join(dir, "gateway"))
		}
		return nil, nil
	}()
	if err != nil {
		return nil, err
	}

	return &TestNode{
		Gateway: g,
	}, nil
}
