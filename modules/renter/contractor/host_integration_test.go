package contractor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	modWallet "github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// newTestingWallet is a helper function that creates a ready-to-use wallet
// and mines some coins into it.
func newTestingWallet(testdir string, cs modules.ConsensusSet, tp modules.TransactionPool) (modules.Wallet, error) {
	w, err := modWallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	if !w.Encrypted() {
		_, err = w.Encrypt(key)
		if err != nil {
			return nil, err
		}
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	// give it some money
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := m.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return w, nil
}

// newTestingHost is a helper function that creates a ready-to-use host.
func newTestingHost(testdir string, cs modules.ConsensusSet, tp modules.TransactionPool) (modules.Host, error) {
	w, err := newTestingWallet(testdir, cs, tp)
	if err != nil {
		return nil, err
	}
	return host.New(cs, tp, w, "localhost:0", filepath.Join(testdir, modules.HostDir))
}

// newTestingContractor is a helper function that creates a ready-to-use
// contractor.
func newTestingContractor(testdir string, cs modules.ConsensusSet, tp modules.TransactionPool) (*Contractor, error) {
	w, err := newTestingWallet(testdir, cs, tp)
	if err != nil {
		return nil, err
	}
	hdb, err := hostdb.New(cs, filepath.Join(testdir, "hostdb"))
	if err != nil {
		return nil, err
	}
	return New(cs, w, tp, hdb, filepath.Join(testdir, "contractor"))
}

// newTestingTrio creates a Host, Contractor, and TestMiner that can be used
// for testing host/renter interactions.
func newTestingTrio(name string) (modules.Host, *Contractor, modules.TestMiner, error) {
	testdir := build.TempDir("contractor", name)

	// create miner
	g, err := gateway.New("localhost:0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, nil, nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, nil, nil, err
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, nil, nil, err
	}
	w, err := modWallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, nil, nil, err
	}
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, nil, nil, err
	}
	if !w.Encrypted() {
		_, err = w.Encrypt(key)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, nil, nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, nil, nil, err
	}

	// create host and contractor, using same consensus set and gateway
	h, err := newTestingHost(filepath.Join(testdir, "Host"), cs, tp)
	if err != nil {
		return nil, nil, nil, err
	}
	c, err := newTestingContractor(filepath.Join(testdir, "Contractor"), cs, tp)
	if err != nil {
		return nil, nil, nil, err
	}

	return h, c, m, nil
}

// TestIntegrationFormContract tests that the contractor can form contracts
// with the host module.
func TestIntegrationFormContract(t *testing.T) {
	// create host+contractor+miner
	h, c, m, err := newTestingTrio("TestIngrationFormContract")
	if err != nil {
		t.Fatal(err)
	}

	// Configure host to accept contracts
	settings := h.InternalSettings()
	settings.AcceptingContracts = true
	err = h.SetInternalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}

	// announce the host
	err = h.Announce()
	if err != nil {
		t.Fatal(err)
	}

	// mine a block, processing the announcement
	m.AddBlock()

	// wait for hostdb to scan host
	for len(c.hdb.RandomHosts(1, nil)) == 0 {
		time.Sleep(time.Millisecond)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.NetAddress())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.newContract(hostEntry, 64000, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	if contract.IP != h.NetAddress() {
		t.Fatal("bad contract")
	}
}
