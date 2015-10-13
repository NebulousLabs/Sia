package miner

import (
	"bytes"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// A minerTester is the helper object for miner testing.
type minerTester struct {
	gateway   modules.Gateway
	cs        modules.ConsensusSet
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	miner *Miner
}

// createMinerTester creates a minerTester that's ready for use.
func createMinerTester(name string) (*minerTester, error) {
	testdir := build.TempDir(modules.MinerDir, name)

	// Create the modules.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g)
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
	m, err := New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble the minerTester.
	mt := &minerTester{
		gateway:   g,
		cs:        cs,
		tpool:     tp,
		wallet:    w,
		walletKey: key,

		miner: m,
	}

	// Mine until the wallet has money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = m.AddBlock()
		if err != nil {
			return nil, err
		}
	}

	return mt, nil
}

// TestIntegrationMiner creates a miner, mines a few blocks, and checks that
// the wallet balance is updating as the blocks get mined.
func TestIntegrationMiner(t *testing.T) {
	mt, err := createMinerTester("TestMiner")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet has money.
	siacoins, _, _ := mt.wallet.ConfirmedBalance()
	if siacoins.IsZero() {
		t.Error("expecting mining full balance to not be zero")
	}

	// Mine a bunch of blocks.
	if testing.Short() {
		t.SkipNow()
	}
	for i := 0; i < 50; i++ {
		b, _ := mt.miner.FindBlock()
		err = mt.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestIntegrationNilMinerDependencies tests that the miner properly handles
// nil inputs for its dependencies.
func TestIntegrationNilMinerDependencies(t *testing.T) {
	mt, err := createMinerTester("TestIntegrationNilMinerDependencies")
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(mt.cs, mt.tpool, nil, "")
	if err != errNilWallet {
		t.Fatal(err)
	}
	_, err = New(mt.cs, nil, mt.wallet, "")
	if err != errNilTpool {
		t.Fatal(err)
	}
	_, err = New(nil, mt.tpool, mt.wallet, "")
	if err != errNilCS {
		t.Fatal(err)
	}
	_, err = New(nil, nil, nil, "")
	if err == nil {
		t.Fatal(err)
	}
}

// TestIntegrationBlocksMined checks that the BlocksMined function correctly
// indicates the number of real blocks and stale blocks that have been mined.
func TestIntegrationBlocksMined(t *testing.T) {
	mt, err := createMinerTester("TestIntegrationBlocksMined")
	if err != nil {
		t.Fatal(err)
	}

	// Get an unsolved header.
	unsolvedHeader, target, err := mt.miner.HeaderForWork()
	if err != nil {
		t.Fatal(err)
	}
	// Unsolve the header.
	for {
		unsolvedHeader.Nonce[0]++
		id := crypto.HashObject(unsolvedHeader)
		if bytes.Compare(target[:], id[:]) < 0 {
			break
		}
	}

	// Get two solved headers.
	header1, target, err := mt.miner.HeaderForWork()
	if err != nil {
		t.Fatal(err)
	}
	header1 = solveHeader(header1, target)
	header2, target, err := mt.miner.HeaderForWork()
	if err != nil {
		t.Fatal(err)
	}
	header2 = solveHeader(header2, target)

	// Submit the unsolved header followed by the two solved headers, this
	// should result in 1 real block mined and 1 stale block mined.
	err = mt.miner.SubmitHeader(unsolvedHeader)
	if err != modules.ErrBlockUnsolved {
		t.Fatal(err)
	}
	err = mt.miner.SubmitHeader(header1)
	if err != nil {
		t.Fatal(err)
	}
	err = mt.miner.SubmitHeader(header2)
	if err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}

	goodBlocks, staleBlocks := mt.miner.BlocksMined()
	if goodBlocks != 1 {
		t.Error("expexting 1 good block")
	}
	if staleBlocks != 1 {
		t.Error(len(mt.miner.blocksFound))
		t.Error("expecting 1 stale block, got", staleBlocks)
	}
}
