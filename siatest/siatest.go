// Package siatest contains a bunch of testing utilities and helper functions.
// Siatest is able to spin up full nodes, groups of full nodes, and then
// provides helper functions for interacting with the full nodes, such as
// mining a block or getting started with renting + hosting.
package siatest

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
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
	// Omissions - if the omit flag is set for a module, that module will not
	// be included in the test node.
	//
	// If you omit a module, you will implicitly omit all dependencies as well.
	OmitConsensusSet    bool
	OmitGateway         bool
	OmitHost            bool
	OmitMiner           bool
	OmitRenter          bool
	OmitTransactionPool bool
	OmitWallet          bool

	// NOTE: if the explorer is ever implemented, it should be omitted by
	// default, since it is not needed for the vast majority of integration
	// tests, and is also very expensive and slow.

	// Custom modules - if the modules is provided directly, the provided
	// module will be used instead of creating a new one. If a custom module is
	// provided, the 'omit' flag for that module must be set to false (which is
	// the default setting).
	ConsensusSet    modules.ConsensusSet
	Gateway         modules.Gateway
	Host            modules.Host
	Miner           modules.TestMiner
	Renter          modules.Renter
	TransactionPool modules.TransactionPool
	Wallet          modules.Wallet

	// Utilities.
	Dir string // The directory to use when creating or opening modules.
}

// TestNode contains all modules, and can be used as a testing node. Modules
// can individually be enabled or disabled, and there are lots of helper
// functions associated with the test node to assist with testing.
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

	// Folder on disk that stores all of the module persistence.
	Client     *api.Client // A client that can be used to talk to this node's API.
	PersistDir string      // The folder used for this node's persist files.
	Server     *api.Server // The server that listens and handles api requests.
}

// NewTestNode will create a new test node. The inputs to the function are the
// respective 'New' calls for each module. We need to use this awkward method
// of initialization because the siatest package cannot import any of the
// modules directly (so that the modules may use the siatest package to test
// themselves).
func NewTestNode(params NewTestNodeParams) (*TestNode, error) {
	dir := params.Dir

	// Gateway.
	g, err := func() (modules.Gateway, error) {
		if !params.OmitGateway && params.Gateway != nil {
			return nil, errors.New("cannot both create a gateway and use a passed in gateway")
		}
		if params.Gateway != nil {
			return params.Gateway, nil
		}
		if params.OmitGateway {
			return nil, nil
		}
		return gateway.New("localhost:0", false, filepath.Join(dir, modules.GatewayDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create gateway"))
	}

	// Consensus.
	cs, err := func() (modules.ConsensusSet, error) {
		if !params.OmitConsensusSet && params.ConsensusSet != nil {
			return nil, errors.New("cannot both create consensus and use passed in consensus")
		}
		if params.ConsensusSet != nil {
			return params.ConsensusSet, nil
		}
		if params.OmitGateway || params.OmitConsensusSet {
			return nil, nil
		}
		return consensus.New(g, false, filepath.Join(dir, modules.ConsensusDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create consensus set"))
	}

	// Transaction Pool.
	tp, err := func() (modules.TransactionPool, error) {
		if !params.OmitTransactionPool && params.TransactionPool != nil {
			return nil, errors.New("cannot create transaction pool and also use custom transaction pool")
		}
		if params.TransactionPool != nil {
			return params.TransactionPool, nil
		}
		if params.OmitGateway || params.OmitConsensusSet || params.OmitTransactionPool {
			return nil, nil
		}
		return transactionpool.New(cs, g, filepath.Join(dir, modules.TransactionPoolDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create transaction pool"))
	}

	// Wallet.
	w, err := func() (modules.Wallet, error) {
		if !params.OmitWallet && params.Wallet != nil {
			return nil, errors.New("cannot create wallet and use custom wallet")
		}
		if params.Wallet != nil {
			return params.Wallet, nil
		}
		if params.OmitGateway || params.OmitConsensusSet || params.OmitTransactionPool || params.OmitWallet {
			return nil, nil
		}
		return wallet.New(cs, tp, filepath.Join(dir, modules.WalletDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create wallet"))
	}

	// Host.
	h, err := func() (modules.Host, error) {
		if !params.OmitHost && params.Host != nil {
			return nil, errors.New("cannot create host and use custom host")
		}
		if params.Host != nil {
			return params.Host, nil
		}
		if params.OmitGateway || params.OmitConsensusSet || params.OmitTransactionPool || params.OmitWallet || params.OmitHost {
			return nil, nil
		}
		return host.New(cs, tp, w, "localhost:0", filepath.Join(dir, modules.HostDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create host"))
	}

	// Renter.
	r, err := func() (modules.Renter, error) {
		if !params.OmitRenter && params.Renter != nil {
			return nil, errors.New("cannot create renter and also use custom renter")
		}
		if params.Renter != nil {
			return params.Renter, nil
		}
		if params.OmitGateway || params.OmitConsensusSet || params.OmitTransactionPool || params.OmitWallet || params.OmitRenter {
			return nil, nil
		}
		return renter.New(g, cs, w, tp, filepath.Join(dir, modules.RenterDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create renter"))
	}

	// Miner.
	m, err := func() (modules.TestMiner, error) {
		if !params.OmitMiner && params.Miner != nil {
			return nil, errors.New("cannot create miner and also use custom miner")
		}
		if params.Miner != nil {
			return params.Miner, nil
		}
		if params.OmitGateway || params.OmitConsensusSet || params.OmitTransactionPool || params.OmitWallet || params.OmitMiner {
			return nil, nil
		}
		return miner.New(cs, tp, w, filepath.Join(dir, modules.MinerDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create miner"))
	}

	// Create the server.
	server, err := api.NewServer("", "", cs, e, g, h, m, r, tp, w)
	if err != nil {
		return nil, errors.AddContext(err, "unable to create server")
	}

	return &TestNode{
		ConsensusSet:    cs,
		Gateway:         g,
		Host:            h,
		Miner:           m,
		Renter:          r,
		TransactionPool: tp,
		Wallet:          w,

		PersistDir: dir,
		Server:     server,
	}, nil
}
