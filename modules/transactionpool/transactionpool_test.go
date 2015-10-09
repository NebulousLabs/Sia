package transactionpool

import (
	"crypto/rand"
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
}

// createTpoolTester returns a ready-to-use tpool tester, with all modules
// initialized.
func createTpoolTester(name string) (*tpoolTester, error) {
	// Initialize the modules.
	testdir := build.TempDir(modules.TransactionPoolDir, name)
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := New(cs, g)
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	var key crypto.TwofishKey
	_, err = rand.Read(key[:])
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
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assebmle all of the objects in to a tpoolTester
	tpt := &tpoolTester{
		cs:        cs,
		gateway:   g,
		tpool:     tp,
		miner:     m,
		wallet:    w,
		walletKey: key,
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

// TestIntegrationNewNilInputs tries to trigger a panic with nil inputs.
func TestIntegrationNewNilInputs(t *testing.T) {
	// Create a gateway and consensus set.
	testdir := build.TempDir(modules.TransactionPoolDir, "TestNewNilInputs")
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Try all combinations of nil inputs.
	_, err = New(nil, nil)
	if err == nil {
		t.Error(err)
	}
	_, err = New(nil, g)
	if err != errNilCS {
		t.Error(err)
	}
	_, err = New(cs, nil)
	if err != errNilGateway {
		t.Error(err)
	}
	_, err = New(cs, g)
	if err != nil {
		t.Error(err)
	}
}
