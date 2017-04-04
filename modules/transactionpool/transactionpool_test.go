package transactionpool

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// A tpoolTester is used during testing to initialize a transaction pool and
// useful helper modules.
type tpoolTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	tpool     *TransactionPool
	miner     modules.TestMiner
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	persistDir string
}

// createTpoolTester returns a ready-to-use tpool tester, with all modules
// initialized.
func createTpoolTester(name string) (*tpoolTester, error) {
	// Initialize the modules.
	testdir := build.TempDir(modules.TransactionPoolDir, name)
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	var key crypto.TwofishKey
	fastrand.Read(key[:])
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all of the objects into a tpoolTester
	tpt := &tpoolTester{
		cs:        cs,
		gateway:   g,
		tpool:     tp,
		miner:     m,
		wallet:    w,
		walletKey: key,

		persistDir: testdir,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := tpt.miner.FindBlock()
		err = tpt.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
	}

	return tpt, nil
}

// Close safely closes the tpoolTester, calling a panic in the event of an
// error since there isn't a good way to errcheck when deferring a Close.
func (tpt *tpoolTester) Close() error {
	errs := []error{
		tpt.cs.Close(),
		tpt.gateway.Close(),
		tpt.tpool.Close(),
		tpt.miner.Close(),
		// TODO: implement modules.Wallet.Close()
		// tpt.wallet.Close()
	}
	if err := build.JoinErrors(errs, "; "); err != nil {
		panic(err)
	}
	return nil
}

// TestIntegrationNewNilInputs tries to trigger a panic with nil inputs.
func TestIntegrationNewNilInputs(t *testing.T) {
	// Create a gateway and consensus set.
	testdir := build.TempDir(modules.TransactionPoolDir, t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tpDir := filepath.Join(testdir, modules.TransactionPoolDir)

	// Try all combinations of nil inputs.
	_, err = New(nil, nil, tpDir)
	if err == nil {
		t.Error(err)
	}
	_, err = New(nil, g, tpDir)
	if err != errNilCS {
		t.Error(err)
	}
	_, err = New(cs, nil, tpDir)
	if err != errNilGateway {
		t.Error(err)
	}
	_, err = New(cs, g, tpDir)
	if err != nil {
		t.Error(err)
	}
}
