package contractor

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// These interfaces define the HostDB's dependencies. Using the smallest
// interface possible makes it easier to mock these dependencies in testing.
type (
	consensusSet interface {
		ConsensusSetSubscribe(modules.ConsensusSetSubscriber, modules.ConsensusChangeID, <-chan struct{}) error
		Synced() bool
		Unsubscribe(modules.ConsensusSetSubscriber)
	}
	// In order to restrict the modules.TransactionBuilder interface, we must
	// provide a shim to bridge the gap between modules.Wallet and
	// transactionBuilder.
	walletShim interface {
		NextAddress() (types.UnlockConditions, error)
		StartTransaction() (modules.TransactionBuilder, error)
	}
	wallet interface {
		NextAddress() (types.UnlockConditions, error)
		StartTransaction() (transactionBuilder, error)
	}
	transactionBuilder interface {
		AddArbitraryData([]byte) uint64
		AddFileContract(types.FileContract) uint64
		AddMinerFee(types.Currency) uint64
		AddParents([]types.Transaction)
		AddSiacoinInput(types.SiacoinInput) uint64
		AddSiacoinOutput(types.SiacoinOutput) uint64
		AddTransactionSignature(types.TransactionSignature) uint64
		Drop()
		FundSiacoins(types.Currency) error
		Sign(bool) ([]types.Transaction, error)
		UnconfirmedParents() ([]types.Transaction, error)
		View() (types.Transaction, []types.Transaction)
		ViewAdded() (parents, coins, funds, signatures []int)
	}
	transactionPool interface {
		AcceptTransactionSet([]types.Transaction) error
		FeeEstimation() (min types.Currency, max types.Currency)
	}

	hostDB interface {
		AllHosts() []modules.HostDBEntry
		ActiveHosts() []modules.HostDBEntry
		Host(types.SiaPublicKey) (modules.HostDBEntry, bool)
		IncrementSuccessfulInteractions(key types.SiaPublicKey)
		IncrementFailedInteractions(key types.SiaPublicKey)
		RandomHosts(n int, exclude []types.SiaPublicKey) ([]modules.HostDBEntry, error)
		ScoreBreakdown(modules.HostDBEntry) modules.HostScoreBreakdown
	}

	persister interface {
		save(contractorPersist) error
		load(*contractorPersist) error
	}
)

// WalletBridge is a bridge for the wallet because wallet is not directly
// compatible with modules.Wallet (wrong type signature for StartTransaction),
// we must provide a bridge type.
type WalletBridge struct {
	W walletShim
}

// NextAddress computes and returns the next address of the wallet.
func (ws *WalletBridge) NextAddress() (types.UnlockConditions, error) { return ws.W.NextAddress() }

// StartTransaction creates a new transactionBuilder that can be used to create
// and sign a transaction.
func (ws *WalletBridge) StartTransaction() (transactionBuilder, error) { return ws.W.StartTransaction() }

// stdPersist implements the persister interface. The filename required by
// these functions is internal to stdPersist.
type stdPersist struct {
	filename string
}

var persistMeta = persist.Metadata{
	Header:  "Contractor Persistence",
	Version: "1.3.1",
}

func (p *stdPersist) save(data contractorPersist) error {
	return persist.SaveJSON(persistMeta, data, p.filename)
}

func (p *stdPersist) load(data *contractorPersist) error {
	return persist.LoadJSON(persistMeta, &data, p.filename)
}

// NewPersist create a new stdPersist.
func NewPersist(dir string) *stdPersist {
	return &stdPersist{
		filename: filepath.Join(dir, "contractor.json"),
	}
}
