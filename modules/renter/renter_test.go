package renter

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// renterTester contains all of the modules that are used while testing the renter.
type renterTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	renter *Renter
}

// Close shuts down the renter tester.
func (rt *renterTester) Close() error {
	rt.wallet.Lock()
	rt.cs.Close()
	rt.gateway.Close()
	return nil
}

// newRenterTester creates a ready-to-use renter tester with money in the
// wallet.
func newRenterTester(name string) (*renterTester, error) {
	// Create the modules.
	testdir := build.TempDir("renter", name)
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	r, err := New(cs, w, tp, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all pieces into a renter tester.
	rt := &renterTester{
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		renter: r,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := rt.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return rt, nil
}

// newContractorTester creates a renterTester, but with the supplied
// hostContractor.
func newContractorTester(name string, hdb hostDB, hc hostContractor) (*renterTester, error) {
	// Create the modules.
	testdir := build.TempDir("renter", name)
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	r, err := newRenter(cs, tp, hdb, hc, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all pieces into a renter tester.
	rt := &renterTester{
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		renter: r,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := rt.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return rt, nil
}

// stubHostDB is the minimal implementation of the hostDB interface. It can be
// embedded in other mock hostDB types, removing the need to reimplement all
// of the hostDB's methods on every mock.
type stubHostDB struct{}

func (stubHostDB) ActiveHosts() []modules.HostDBEntry   { return nil }
func (stubHostDB) AllHosts() []modules.HostDBEntry      { return nil }
func (stubHostDB) AverageContractPrice() types.Currency { return types.Currency{} }
func (stubHostDB) Close() error                         { return nil }
func (stubHostDB) IsOffline(modules.NetAddress) bool    { return true }

// stubContractor is the minimal implementation of the hostContractor
// interface.
type stubContractor struct{}

func (stubContractor) SetAllowance(modules.Allowance) error { return nil }
func (stubContractor) Allowance() modules.Allowance         { return modules.Allowance{} }
func (stubContractor) Contract(modules.NetAddress) (modules.RenterContract, bool) {
	return modules.RenterContract{}, false
}
func (stubContractor) Contracts() []modules.RenterContract                    { return nil }
func (stubContractor) IsOffline(modules.NetAddress) bool                      { return false }
func (stubContractor) Editor(types.FileContractID) (contractor.Editor, error) { return nil, nil }
func (stubContractor) Downloader(types.FileContractID) (contractor.Downloader, error) {
	return nil, nil
}
func (stubContractor) Metrics() (m modules.RenterFinancialMetrics, cs []modules.RenterContractMetrics) {
	return
}
