package wallet

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// A Wallet tester contains a ConsensusTester and has a bunch of helpful
// functions for facilitating wallet integration testing.
type walletTester struct {
	cs      modules.ConsensusSet
	gateway modules.Gateway
	tpool   modules.TransactionPool
	miner   modules.TestMiner
	wallet  *Wallet

	walletMasterKey crypto.TwofishKey

	persistDir string
}

// createWalletTester takes a testing.T and creates a WalletTester.
func createWalletTester(name string) (*walletTester, error) {
	// Create the modules
	testdir := build.TempDir(modules.WalletDir, name)
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
	w, err := New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	var masterKey crypto.TwofishKey
	fastrand.Read(masterKey[:])
	_, err = w.Encrypt(masterKey)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(masterKey)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}

	// Assemble all components into a wallet tester.
	wt := &walletTester{
		cs:      cs,
		gateway: g,
		tpool:   tp,
		miner:   m,
		wallet:  w,

		walletMasterKey: masterKey,

		persistDir: testdir,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := wt.miner.FindBlock()
		err := wt.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
	}
	return wt, nil
}

// createBlankWalletTester creates a wallet tester that has not mined any
// blocks or encrypted the wallet.
func createBlankWalletTester(name string) (*walletTester, error) {
	// Create the modules
	testdir := build.TempDir(modules.WalletDir, name)
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
	w, err := New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all components into a wallet tester.
	wt := &walletTester{
		gateway: g,
		cs:      cs,
		tpool:   tp,
		miner:   m,
		wallet:  w,

		persistDir: testdir,
	}
	return wt, nil
}

// closeWt closes all of the modules in the wallet tester.
func (wt *walletTester) closeWt() error {
	errs := []error{
		wt.gateway.Close(),
		wt.cs.Close(),
		wt.tpool.Close(),
		wt.miner.Close(),
		wt.wallet.Close(),
	}
	return build.JoinErrors(errs, "; ")
}

// TestNilInputs tries starting the wallet using nil inputs.
func TestNilInputs(t *testing.T) {
	testdir := build.TempDir(modules.WalletDir, t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal(err)
	}

	wdir := filepath.Join(testdir, modules.WalletDir)
	_, err = New(cs, nil, wdir)
	if err != errNilTpool {
		t.Error(err)
	}
	_, err = New(nil, tp, wdir)
	if err != errNilConsensusSet {
		t.Error(err)
	}
	_, err = New(nil, nil, wdir)
	if err != errNilConsensusSet {
		t.Error(err)
	}
}

// TestAllAddresses checks that AllAddresses returns all of the wallet's
// addresses in sorted order.
func TestAllAddresses(t *testing.T) {
	wt, err := createBlankWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	wt.wallet.keys[types.UnlockHash{1}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{5}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{0}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{2}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{4}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{3}] = spendableKey{}
	addrs := wt.wallet.AllAddresses()
	for i := range addrs {
		if addrs[i][0] != byte(i) {
			t.Error("address sorting failed:", i, addrs[i][0])
		}
	}
}

// TestCloseWallet tries to close the wallet.
func TestCloseWallet(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	testdir := build.TempDir(modules.WalletDir, t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal(err)
	}
	wdir := filepath.Join(testdir, modules.WalletDir)
	w, err := New(cs, tp, wdir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestRescanning verifies that calling Rescanning during a scan operation
// returns true, and false otherwise.
func TestRescanning(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// A fresh wallet should not be rescanning.
	if wt.wallet.Rescanning() {
		t.Fatal("fresh wallet should not report that a scan is underway")
	}

	// lock the wallet
	wt.wallet.Lock()

	// spawn an unlock goroutine
	errChan := make(chan error)
	go func() {
		// acquire the write lock so that Unlock acquires the trymutex, but
		// cannot proceed further
		wt.wallet.mu.Lock()
		errChan <- wt.wallet.Unlock(wt.walletMasterKey)
	}()

	// wait for goroutine to start, after which Rescanning should return true
	time.Sleep(time.Millisecond * 10)
	if !wt.wallet.Rescanning() {
		t.Fatal("wallet should report that a scan is underway")
	}

	// release the mutex and allow the call to complete
	wt.wallet.mu.Unlock()
	if err := <-errChan; err != nil {
		t.Fatal("unlock failed:", err)
	}

	// Rescanning should now return false again
	if wt.wallet.Rescanning() {
		t.Fatal("wallet should not report that a scan is underway")
	}
}

// TestFutureAddressGeneration checks if the right amount of future addresses
// is generated after calling NextAddress() or locking + unlocking the wallet.
func TestFutureAddressGeneration(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Check if number of future keys is correct
	wt.wallet.mu.RLock()
	progress, err := dbGetPrimarySeedProgress(wt.wallet.dbTx)
	wt.wallet.mu.RUnlock()
	if err != nil {
		t.Fatal("Couldn't fetch primary seed from db")
	}

	actualKeys := uint64(len(wt.wallet.futureKeys))
	expectedKeys := maxFutureKeys(progress)
	if actualKeys != expectedKeys {
		t.Errorf("expected len(futureKeys) == %d but was %d", actualKeys, expectedKeys)
	}

	// Generate some more keys
	for i := 0; i < 100; i++ {
		wt.wallet.NextAddress()
	}

	// Lock and unlock
	wt.wallet.Lock()
	wt.wallet.Unlock(wt.walletMasterKey)

	wt.wallet.mu.RLock()
	progress, err = dbGetPrimarySeedProgress(wt.wallet.dbTx)
	wt.wallet.mu.RUnlock()
	if err != nil {
		t.Fatal("Couldn't fetch primary seed from db")
	}

	actualKeys = uint64(len(wt.wallet.futureKeys))
	expectedKeys = maxFutureKeys(progress)
	if actualKeys != expectedKeys {
		t.Errorf("expected len(futureKeys) == %d but was %d", actualKeys, expectedKeys)
	}

	wt.wallet.mu.RLock()
	for i := range wt.wallet.keys {
		_, exists := wt.wallet.futureKeys[i]
		if exists {
			t.Fatal("wallet.keys contained a key which is also present in wallet.futurekeys")
		}
	}
	wt.wallet.mu.RUnlock()
}

// TestFutureAddressReceive
func TestFutureAddressReceive(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	builder := wt.wallet.StartTransaction()
	payout := types.ZeroCurrency

	// choose 10 of the future keys and remember them
	var receivingAddresses []types.UnlockHash
	for uh := range wt.wallet.futureKeys {
		sco := types.SiacoinOutput{
			UnlockHash: uh,
			Value:      types.NewCurrency64(1e3),
		}

		builder.AddSiacoinOutput(sco)
		payout = payout.Add(sco.Value)
		receivingAddresses = append(receivingAddresses, uh)

		if len(receivingAddresses) > 10 {
			break
		}
	}

	err = builder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}

	tSet, err := builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	err = wt.tpool.AcceptTransactionSet(tSet)
	if err != nil {
		t.Fatal(err)
	}

	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check if the receiving addresses were moved from future keys to keys
	wt.wallet.mu.RLock()
	for _, uh := range receivingAddresses {
		_, exists := wt.wallet.futureKeys[uh]
		if exists {
			t.Fatal("UnlockHash still exists in wallet.futureKeys")
		}

		_, exists = wt.wallet.keys[uh]
		if !exists {
			t.Fatal("UnlockHash not in map of spendable keys")
		}
	}
	wt.wallet.mu.RUnlock()
}
