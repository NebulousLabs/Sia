// Package node provides tooling for creating a Sia node. Sia nodes consist of a
// collection of modules. The node package gives you tools to easily assemble
// various combinations of modules with varying dependencies and settings,
// including templates for assembling sane no-hassle Sia nodes.
package node

// TODO: Add support for the explorer.

// TODO: Add support for custom dependencies and parameters for all of the
// modules.

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/persist"

	"github.com/NebulousLabs/errors"
)

// NodeParams contains a bunch of parameters for creating a new test node. As
// there are many options, templates are provided that you can modify which
// cover the most common use cases.
//
// Each module is created separately. There are several ways to create a module,
// though not all methods are currently available for each module. You should
// only use one method for creating a module, using multiple methods will cause
// an error.
//		+ Indicate with the 'CreateModule' bool that a module should be created
//		  automatically. To create the module with custom dependencies, pass the
//		  custom dependencies in using the 'ModuleDependencies' field.
//		+ Pass an existing module in directly.
//		+ Set 'CreateModule' to false and do not pass in an existing module.
//		  This will result in a 'nil' module, meaning the node will not have
//		  that module.
type NodeParams struct {
	// Flags to indicate which modules should be created automatically by the
	// server. If you are providing a pre-existing module, do not set the flag
	// for that module.
	//
	// NOTE / TODO: The code does not currently enforce this, but you should not
	// provide a custom module unless all of its dependencies are also custom.
	// Example: if the ConsensusSet is custom, the Gateway should also be
	// custom. The TransactionPool however does not need to be custom in this
	// example.
	CreateConsensusSet    bool
	CreateExplorer        bool
	CreateGateway         bool
	CreateHost            bool
	CreateMiner           bool
	CreateRenter          bool
	CreateTransactionPool bool
	CreateWallet          bool

	// Custom modules - if the modules is provided directly, the provided
	// module will be used instead of creating a new one. If a custom module is
	// provided, the 'omit' flag for that module must be set to false (which is
	// the default setting).
	ConsensusSet    modules.ConsensusSet
	Explorer        modules.Explorer
	Gateway         modules.Gateway
	Host            modules.Host
	Miner           modules.TestMiner
	Renter          modules.Renter
	TransactionPool modules.TransactionPool
	Wallet          modules.Wallet

	// Dependencies for each module supporting dependency injection.
	ContractorDeps  modules.Dependencies
	ContractSetDeps modules.Dependencies
	HostDBDeps      modules.Dependencies
	RenterDeps      modules.Dependencies
	WalletDeps      modules.Dependencies

	// Custom settings for modules
	Allowance modules.Allowance

	// The following fields are used to skip parts of the node set up
	SkipSetAllowance  bool
	SkipHostDiscovery bool

	// The high level directory where all the persistence gets stored for the
	// moudles.
	Dir string
}

// Node is a collection of Sia modules operating together as a Sia node.
type Node struct {
	// The modules of the node. Modules that are not initialized will be nil.
	ConsensusSet    modules.ConsensusSet
	Explorer        modules.Explorer
	Gateway         modules.Gateway
	Host            modules.Host
	Miner           modules.TestMiner
	Renter          modules.Renter
	TransactionPool modules.TransactionPool
	Wallet          modules.Wallet

	// The high level directory where all the persistence gets stored for the
	// moudles.
	Dir string
}

// Close will call close on every module within the node, combining and
// returning the errors.
func (n *Node) Close() (err error) {
	if n.Explorer != nil {
		err = errors.Compose(n.Explorer.Close())
	}
	if n.Miner != nil {
		err = errors.Compose(n.Miner.Close())
	}
	if n.Host != nil {
		err = errors.Compose(n.Host.Close())
	}
	if n.Renter != nil {
		err = errors.Compose(n.Renter.Close())
	}
	if n.Wallet != nil {
		err = errors.Compose(n.Wallet.Close())
	}
	if n.TransactionPool != nil {
		err = errors.Compose(n.TransactionPool.Close())
	}
	if n.ConsensusSet != nil {
		err = errors.Compose(n.ConsensusSet.Close())
	}
	if n.Gateway != nil {
		err = errors.Compose(n.Gateway.Close())
	}
	return err
}

// New will create a new test node. The inputs to the function are the
// respective 'New' calls for each module. We need to use this awkward method
// of initialization because the siatest package cannot import any of the
// modules directly (so that the modules may use the siatest package to test
// themselves).
func New(params NodeParams) (*Node, error) {
	dir := params.Dir

	// Gateway.
	g, err := func() (modules.Gateway, error) {
		if params.CreateGateway && params.Gateway != nil {
			return nil, errors.New("cannot both create a gateway and use a passed in gateway")
		}
		/* Template for dealing with optional dependencies:
		if !params.CreateGateway && parames.GatewayDependencies != nil {
			return nil, errors.New("cannot pass in gateway dependencies if you are not creating a gateway")
		}
		*/
		if params.Gateway != nil {
			return params.Gateway, nil
		}
		if !params.CreateGateway {
			return nil, nil
		}
		/* Template for dealing with optional dependencies:
		if params.GatewayDependencies == nil {
			gateway.New(...
		} else {
			gateway.NewDeps(...
		}
		*/
		return gateway.New("localhost:0", false, filepath.Join(dir, modules.GatewayDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create gateway"))
	}

	// Consensus.
	cs, err := func() (modules.ConsensusSet, error) {
		if params.CreateConsensusSet && params.ConsensusSet != nil {
			return nil, errors.New("cannot both create consensus and use passed in consensus")
		}
		if params.ConsensusSet != nil {
			return params.ConsensusSet, nil
		}
		if !params.CreateConsensusSet {
			return nil, nil
		}
		return consensus.New(g, false, filepath.Join(dir, modules.ConsensusDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create consensus set"))
	}

	// Transaction Pool.
	tp, err := func() (modules.TransactionPool, error) {
		if params.CreateTransactionPool && params.TransactionPool != nil {
			return nil, errors.New("cannot create transaction pool and also use custom transaction pool")
		}
		if params.TransactionPool != nil {
			return params.TransactionPool, nil
		}
		if !params.CreateTransactionPool {
			return nil, nil
		}
		return transactionpool.New(cs, g, filepath.Join(dir, modules.TransactionPoolDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create transaction pool"))
	}

	// Wallet.
	w, err := func() (modules.Wallet, error) {
		if params.CreateWallet && params.Wallet != nil {
			return nil, errors.New("cannot create wallet and use custom wallet")
		}
		if params.Wallet != nil {
			return params.Wallet, nil
		}
		if !params.CreateWallet {
			return nil, nil
		}
		walletDeps := params.WalletDeps
		if walletDeps == nil {
			walletDeps = modules.ProdDependencies
		}
		return wallet.NewCustomWallet(cs, tp, filepath.Join(dir, modules.WalletDir), walletDeps)
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create wallet"))
	}

	// Host.
	h, err := func() (modules.Host, error) {
		if params.CreateHost && params.Host != nil {
			return nil, errors.New("cannot create host and use custom host")
		}
		if params.Host != nil {
			return params.Host, nil
		}
		if !params.CreateHost {
			return nil, nil
		}
		return host.New(cs, tp, w, "localhost:0", filepath.Join(dir, modules.HostDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create host"))
	}

	// Renter.
	r, err := func() (modules.Renter, error) {
		if params.CreateRenter && params.Renter != nil {
			return nil, errors.New("cannot create renter and also use custom renter")
		}
		if params.Renter != nil {
			return params.Renter, nil
		}
		if !params.CreateRenter {
			return nil, nil
		}
		contractorDeps := params.ContractorDeps
		if contractorDeps == nil {
			contractorDeps = modules.ProdDependencies
		}
		contractSetDeps := params.ContractSetDeps
		if contractSetDeps == nil {
			contractSetDeps = modules.ProdDependencies
		}
		hostDBDeps := params.HostDBDeps
		if hostDBDeps == nil {
			hostDBDeps = modules.ProdDependencies
		}
		renterDeps := params.RenterDeps
		if renterDeps == nil {
			renterDeps = modules.ProdDependencies
		}
		persistDir := filepath.Join(dir, modules.RenterDir)

		// HostDB
		hdb, err := hostdb.NewCustomHostDB(g, cs, persistDir, hostDBDeps)
		if err != nil {
			return nil, err
		}
		// ContractSet
		contractSet, err := proto.NewContractSet(filepath.Join(persistDir, "contracts"), contractSetDeps)
		if err != nil {
			return nil, err
		}
		// Contractor
		logger, err := persist.NewFileLogger(filepath.Join(persistDir, "contractor.log"))
		if err != nil {
			return nil, err
		}
		hc, err := contractor.NewCustomContractor(cs, &contractor.WalletBridge{W: w}, tp, hdb, contractSet, contractor.NewPersist(persistDir), logger, contractorDeps)
		if err != nil {
			return nil, err
		}
		return renter.NewCustomRenter(g, cs, tp, hdb, hc, persistDir, renterDeps)
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create renter"))
	}

	// Miner.
	m, err := func() (modules.TestMiner, error) {
		if params.CreateMiner && params.Miner != nil {
			return nil, errors.New("cannot create miner and also use custom miner")
		}
		if params.Miner != nil {
			return params.Miner, nil
		}
		if !params.CreateMiner {
			return nil, nil
		}
		m, err := miner.New(cs, tp, w, filepath.Join(dir, modules.MinerDir))
		if err != nil {
			return nil, err
		}
		return m, nil
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create miner"))
	}

	// Explorer.
	e, err := func() (modules.Explorer, error) {
		if !params.CreateExplorer && params.Explorer != nil {
			return nil, errors.New("cannot create explorer and also use custom explorer")
		}
		if params.Explorer != nil {
			return params.Explorer, nil
		}
		if !params.CreateExplorer {
			return nil, nil
		}
		// TODO: Implement explorer.
		return nil, errors.New("explorer not implemented")
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create explorer"))
	}

	return &Node{
		ConsensusSet:    cs,
		Explorer:        e,
		Gateway:         g,
		Host:            h,
		Miner:           m,
		Renter:          r,
		TransactionPool: tp,
		Wallet:          w,

		Dir: dir,
	}, nil
}
