package node

// templates.go contains a bunch of sane default templates that you can use to
// create Sia nodes.

var (
	// AllModulesTemplate is a template for a Sia node that has all modules
	// enabled.
	AllModulesTemplate = NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false, // TODO: Implement explorer.
		CreateGateway:         true,
		CreateHost:            true,
		CreateMiner:           true,
		CreateRenter:          true,
		CreateTransactionPool: true,
		CreateWallet:          true,
	}
	// ConsensusSetTemplate is a template for a Sia node that has a functioning
	// consensus set. The node has a consensus set and all dependencies, but no
	// other modules.
	ConsensusSetTemplate = NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            false,
		CreateMiner:           false,
		CreateRenter:          false,
		CreateTransactionPool: false,
		CreateWallet:          false,
	}
	// GatewayTemplate is a template for a Sia node that has a functioning
	// gateway. The node has a gateway and all dependencies, but no other
	// modules.
	GatewayTemplate = NodeParams{
		CreateConsensusSet:    false,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            false,
		CreateMiner:           false,
		CreateRenter:          false,
		CreateTransactionPool: false,
		CreateWallet:          false,
	}
	// HostTemplate is a template for a Sia node that has a functioning host.
	// The node has a host and all dependencies, but no other modules.
	HostTemplate = NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            true,
		CreateMiner:           false,
		CreateRenter:          false,
		CreateTransactionPool: true,
		CreateWallet:          true,
	}
	// MinerTemplate is a template for a Sia node that has a functioning miner.
	// The node has a miner and all dependencies, but no other modules.
	MinerTemplate = NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            false,
		CreateMiner:           true,
		CreateRenter:          false,
		CreateTransactionPool: true,
		CreateWallet:          true,
	}
	// RenterTemplate is a template for a Sia node that has a functioning
	// renter. The node has a renter and all dependencies, but no other
	// modules.
	RenterTemplate = NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            false,
		CreateMiner:           false,
		CreateRenter:          true,
		CreateTransactionPool: true,
		CreateWallet:          true,
	}
	// TransactionPoolTemplate is a template for a Sia node that has a
	// functioning transaction pool. The node has a transaction pool and all
	// dependencies, but no other modules.
	TransactionPoolTemplate = NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            false,
		CreateMiner:           false,
		CreateRenter:          false,
		CreateTransactionPool: true,
		CreateWallet:          false,
	}
	// WalletTemplate is a template for a Sia node that has a functioning
	// wallet. The node has a wallet and all dependencies, but no other
	// modules.
	WalletTemplate = NodeParams{
		CreateConsensusSet:    true,
		CreateExplorer:        false,
		CreateGateway:         true,
		CreateHost:            false,
		CreateMiner:           false,
		CreateRenter:          false,
		CreateTransactionPool: true,
		CreateWallet:          true,
	}
)

// AllModules returns an AllModulesTemplate filled out with the provided dir.
func AllModules(dir string) NodeParams {
	template := AllModulesTemplate
	template.Dir = dir
	return template
}

// ConsensusSet returns a HostTemplate filled out with the provided dir.
func ConsensusSet(dir string) NodeParams {
	template := ConsensusSetTemplate
	template.Dir = dir
	return template
}

// Gateway returns a HostTemplate filled out with the provided dir.
func Gateway(dir string) NodeParams {
	template := GatewayTemplate
	template.Dir = dir
	return template
}

// Host returns a HostTemplate filled out with the provided dir.
func Host(dir string) NodeParams {
	template := HostTemplate
	template.Dir = dir
	return template
}

// Miner returns a MinerTemplate filled out with the provided dir.
func Miner(dir string) NodeParams {
	template := MinerTemplate
	template.Dir = dir
	return template
}

// Renter returns a RenterTemplate filled out with the provided dir.
func Renter(dir string) NodeParams {
	template := RenterTemplate
	template.Dir = dir
	return template
}

// TransactionPool returns a TransactionPoolTemplate filled out with the provided dir.
func TransactionPool(dir string) NodeParams {
	template := TransactionPoolTemplate
	template.Dir = dir
	return template
}

// Wallet returns a WalletTemplate filled out with the provided dir.
func Wallet(dir string) NodeParams {
	template := WalletTemplate
	template.Dir = dir
	return template
}
