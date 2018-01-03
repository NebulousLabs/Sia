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

	// WalletTemplate is a template for a Sia node that has a functioning
	// wallet. The node has a wallet and all dependencies, but no other modules.
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

// Wallet returns a WalletTemplate filled out with the provided dir.
func Wallet(dir string) NodeParams {
	template := WalletTemplate
	template.Dir = dir
	return template
}
