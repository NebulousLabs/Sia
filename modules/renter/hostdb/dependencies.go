package hostdb

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// These interfaces define the HostDB's dependencies. Using the smallest
// interface possible makes it easier to mock these dependencies in testing.
type (
	hdbConsensusSet interface {
		ConsensusSetSubscribe(modules.ConsensusSetSubscriber)
	}
	hdbTransactionBuilder interface {
		AddArbitraryData([]byte) uint64
		AddFileContract(types.FileContract) uint64
		Drop()
		FundSiacoins(types.Currency) error
		Sign(bool) ([]types.Transaction, error)
		View() (types.Transaction, []types.Transaction)
	}
	hdbWallet interface {
		NextAddress() (types.UnlockConditions, error)
		StartTransaction() hdbTransactionBuilder
	}
	hdbTransactionPool interface {
		AcceptTransactionSet([]types.Transaction) error
	}

	hdbDialer interface {
		DialTimeout(modules.NetAddress, time.Duration) (net.Conn, error)
	}
)

// because hdbWallet is not directly compatible with modules.Wallet (wrong
// type signature for StartTransaction), we must provide a shim type.
type hdbWalletShim struct {
	w modules.Wallet
}

func (ws *hdbWalletShim) NextAddress() (types.UnlockConditions, error) { return ws.w.NextAddress() }
func (ws *hdbWalletShim) StartTransaction() hdbTransactionBuilder      { return ws.w.StartTransaction() }

type stdDialer struct{}

func (d stdDialer) DialTimeout(addr modules.NetAddress, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", string(addr), timeout)
}
