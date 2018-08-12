package miner

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// A minerTester is the helper object for miner testing.
type minerTester struct {
	gateway   modules.Gateway
	cs        modules.ConsensusSet
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	miner *Miner

	minedBlocks []types.Block
	persistDir  string
}

// createMinerTester creates a minerTester that's ready for use.
func createMinerTester(name string) (*minerTester, error) {
	testdir := build.TempDir(modules.MinerDir, name)

	// Create the modules.
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

		persistDir: testdir,
	}

	// Mine until the wallet has money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, err := m.AddBlock()
		if err != nil {
			return nil, err
		}
		mt.minedBlocks = append(mt.minedBlocks, b)
	}

	return mt, nil
}

// TestIntegrationMiner creates a miner, mines a few blocks, and checks that
// the wallet balance is updating as the blocks get mined.
func TestIntegrationMiner(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet has money.
	siacoins, _, _, err := mt.wallet.ConfirmedBalance()
	if err != nil {
		t.Error(err)
	}
	if siacoins.IsZero() {
		t.Error("expecting mining full balance to not be zero")
	}

	// Mine a bunch of blocks.
	for i := 0; i < 50; i++ {
		b, _ := mt.miner.FindBlock()
		err = mt.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}
	morecoins, _, _, err := mt.wallet.ConfirmedBalance()
	if err != nil {
		t.Error(err)
	}
	if siacoins.Cmp(morecoins) >= 0 {
		t.Error("wallet is not gaining balance while mining")
	}
}

// TestIntegrationNilMinerDependencies tests that the miner properly handles
// nil inputs for its dependencies.
func TestIntegrationNilMinerDependencies(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester(t.Name())
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
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Get an unsolved header.
	unsolvedHeader, target, err := mt.miner.HeaderForWork()
	if err != nil {
		t.Fatal(err)
	}
	// Unsolve the header - necessary because the target is very low when
	// mining.
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
		t.Error("expecting 1 good block")
	}
	if staleBlocks != 1 {
		t.Error("expecting 1 stale block, got", staleBlocks)
	}

	// Reboot the miner and verify that the block record has persisted.
	err = mt.miner.Close()
	if err != nil {
		t.Fatal(err)
	}
	rebootMiner, err := New(mt.cs, mt.tpool, mt.wallet, filepath.Join(mt.persistDir, modules.MinerDir))
	if err != nil {
		t.Fatal(err)
	}
	goodBlocks, staleBlocks = rebootMiner.BlocksMined()
	if goodBlocks != 1 {
		t.Error("expecting 1 good block")
	}
	if staleBlocks != 1 {
		t.Error("expecting 1 stale block, got", staleBlocks)
	}
}

// TestIntegrationAutoRescan triggers a rescan during a call to New and
// verifies that the rescanning happens correctly. The rescan is triggered by
// a call to New, instead of getting called directly.
func TestIntegrationAutoRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	_, err = mt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Get the persist data of the current miner.
	oldChange := mt.miner.persist.RecentChange
	oldHeight := mt.miner.persist.Height
	oldTarget := mt.miner.persist.Target

	// Corrupt the miner, close the miner, and make a new one from the same
	// directory.
	mt.miner.persist.RecentChange[0]++
	mt.miner.persist.Height += 1e5
	mt.miner.persist.Target[0]++
	err = mt.miner.Close() // miner saves when it closes.
	if err != nil {
		t.Fatal(err)
	}

	// Verify that rescanning resolved the corruption in the miner.
	m, err := New(mt.cs, mt.tpool, mt.wallet, filepath.Join(mt.persistDir, modules.MinerDir))
	if err != nil {
		t.Fatal(err)
	}
	// Check that after rescanning, the values have returned to the usual values.
	if m.persist.RecentChange != oldChange {
		t.Error("rescan failed, ended up on the wrong change")
	}
	if m.persist.Height != oldHeight {
		t.Error("rescan failed, ended up at the wrong height")
	}
	if m.persist.Target != oldTarget {
		t.Error("rescan failed, ended up at the wrong target")
	}
}

// TestIntegrationStartupRescan probes the startupRescan function, checking
// that it works in the naive case. Rescan is called directly.
func TestIntegrationStartupRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Check that the miner's persist variables have been initialized to the
	// first few blocks.
	if mt.miner.persist.RecentChange == (modules.ConsensusChangeID{}) || mt.miner.persist.Height == 0 || mt.miner.persist.Target == (types.Target{}) {
		t.Fatal("miner persist variables not initialized")
	}
	oldChange := mt.miner.persist.RecentChange
	oldHeight := mt.miner.persist.Height
	oldTarget := mt.miner.persist.Target

	// Corrupt the miner and verify that a rescan repairs the corruption.
	mt.miner.persist.RecentChange[0]++
	mt.miner.persist.Height += 500
	mt.miner.persist.Target[0]++
	mt.cs.Unsubscribe(mt.miner)
	err = mt.miner.startupRescan()
	if err != nil {
		t.Fatal(err)
	}
	if mt.miner.persist.RecentChange != oldChange {
		t.Error("rescan failed, ended up on the wrong change")
	}
	if mt.miner.persist.Height != oldHeight {
		t.Error("rescan failed, ended up at the wrong height")
	}
	if mt.miner.persist.Target != oldTarget {
		t.Error("rescan failed, ended up at the wrong target")
	}
}

// TestMinerCloseDeadlock checks that the miner can cleanly close even if the
// CPU miner is running.
func TestMinerCloseDeadlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	// StartCPUMining calls `go threadedMine()`, which needs to access the miner
	// before Close() does in the next goroutine, otherwise m.tg.Add() fails
	// at the top of threadedMine() and threadedMine() exits (silently!).
	// I haven't seen this behavior since sticking Close() inside a goroutine,
	// but I'm not sure that's comfort enough.
	mt.miner.StartCPUMining()
	time.Sleep(time.Millisecond * 250)

	closed := make(chan struct{})
	go func() {
		if err := mt.miner.Close(); err != nil {
			t.Fatal(err)
		}
		closed <- struct{}{}
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("mt.miner.Close never completed")
	}
}
