package contractor

import (
	"net"
	"path/filepath"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// These interfaces define the HostDB's dependencies. Using the smallest
// interface possible makes it easier to mock these dependencies in testing.
type (
	consensusSet interface {
		ConsensusSetPersistentSubscribe(modules.ConsensusSetSubscriber, modules.ConsensusChangeID) error
	}
	// in order to restrict the modules.TransactionBuilder interface, we must
	// provide a shim to bridge the gap between modules.Wallet and
	// transactionBuilder.
	walletShim interface {
		NextAddress() (types.UnlockConditions, error)
		StartTransaction() modules.TransactionBuilder
	}
	wallet interface {
		NextAddress() (types.UnlockConditions, error)
		StartTransaction() transactionBuilder
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
		View() (types.Transaction, []types.Transaction)
		ViewAdded() (parents, coins, funds, signatures []int)
	}
	transactionPool interface {
		AcceptTransactionSet([]types.Transaction) error
		FeeEstimation() (min types.Currency, max types.Currency)
	}

	hostDB interface {
		Host(modules.NetAddress) (modules.HostDBEntry, bool)
		RandomHosts(n int, exclude []modules.NetAddress) []modules.HostDBEntry
	}

	dialer interface {
		DialTimeout(modules.NetAddress, time.Duration) (net.Conn, error)
	}

	persister interface {
		save(contractorPersist) error
		load(*contractorPersist) error
	}

	logger interface {
		Println(...interface{})
	}
)

// because wallet is not directly compatible with modules.Wallet (wrong
// type signature for StartTransaction), we must provide a bridge type.
type walletBridge struct {
	w walletShim
}

func (ws *walletBridge) NextAddress() (types.UnlockConditions, error) { return ws.w.NextAddress() }
func (ws *walletBridge) StartTransaction() transactionBuilder         { return ws.w.StartTransaction() }

// stdDialer implements the dialer interface via net.DialTimeout.
type stdDialer struct{}

func (d stdDialer) DialTimeout(addr modules.NetAddress, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", string(addr), timeout)
}

// stdPersist implements the persister interface via persist.SaveFile and
// persist.LoadFile. The metadata and filename required by these functions is
// internal to stdPersist.
type stdPersist struct {
	meta     persist.Metadata
	filename string
}

func (p *stdPersist) save(data contractorPersist) error {
	return persist.SaveFile(p.meta, data, p.filename)
}

func (p *stdPersist) load(data *contractorPersist) error {
	return persist.LoadFile(p.meta, data, p.filename)
}

func newPersist(dir string) *stdPersist {
	return &stdPersist{
		meta: persist.Metadata{
			Header:  "Contractor Persistence",
			Version: "0.5.2",
		},
		filename: filepath.Join(dir, "contractor.json"),
	}
}

// newLogger creates a persist.Logger with the standard filename.
func newLogger(dir string) (*persist.Logger, error) {
	return persist.NewLogger(filepath.Join(dir, "contractor.log"))
}
