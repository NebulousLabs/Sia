package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// A hostTester is the helper object for host testing, including helper modules
// and methods for controlling synchronization.
type hostTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	host *Host

	persistDir string
}

// initWallet creates a wallet key, initializes the host wallet, unlocks it,
// and then stores the key in the host tester.
func (ht *hostTester) initWallet() error {
	// Create the keys for the wallet and unlock it.
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return err
	}
	ht.walletKey = key
	_, err = ht.wallet.Encrypt(key)
	if err != nil {
		return err
	}
	err = ht.wallet.Unlock(key)
	if err != nil {
		return err
	}
	return nil
}

// blankHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankHostTester(name string) (*hostTester, error) {
	testdir := build.TempDir(modules.HostDir, name)

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
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	h, err := New(cs, tp, w, ":0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}

	// Assemble all objects into a hostTester
	ht := &hostTester{
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		host: h,

		persistDir: testdir,
	}

	return ht, nil
}

// newHostTester creates a host tester with an initialized wallet and money in
// that wallet.
func newHostTester(name string) (*hostTester, error) {
	// Create a blank host tester.
	ht, err := blankHostTester(name)
	if err != nil {
		return nil, err
	}

	// Initialize the wallet and mine blocks until the wallet has money.
	err = ht.initWallet()
	if err != nil {
		return nil, err
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return ht, nil
}

// TestHostInitialization checks that the host intializes to sensisble default
// values.
func TestHostInitialization(t *testing.T) {
	// Create a blank host tester and check that the height is zero.
	bht, err := blankHostTester("TestHostInitialization")
	if err != nil {
		t.Fatal(err)
	}
	if bht.host.blockHeight != 0 {
		t.Error("host initialized to the wrong block height")
	}

	// Initialize the wallet so that a block can be mined, then mine a block
	// and check that it sets the host height to 1.
	err = bht.initWallet()
	if err != nil {
		t.Fatal(err)
	}
	_, err = bht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if bht.host.blockHeight != 1 {
		t.Fatal("block height did not increase correctly after first block mined")
	}
}

// TestNilValues tries initializing the host with nil values.
func TestNilValues(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestStartupRescan")
	if err != nil {
		t.Fatal(err)
	}

	hostDir := filepath.Join(ht.persistDir, modules.HostDir)
	_, err = New(nil, ht.tpool, ht.wallet, ":0", hostDir)
	if err != errNilCS {
		t.Fatal("could not trigger errNilCS")
	}
	_, err = New(ht.cs, nil, ht.wallet, ":0", hostDir)
	if err != errNilTpool {
		t.Fatal("could not trigger errNilTpool")
	}
	_, err = New(ht.cs, ht.tpool, nil, ":0", hostDir)
	if err != errNilWallet {
		t.Fatal("Could not trigger errNilWallet")
	}
}

// TestStartupRescan probes the startupRescan function, verifying that it
// works in the naive case. The rescan is triggered manually.
func TestStartupRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestStartupRescan")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the host's persistent variables have incorporated the first
	// few blocks.
	if ht.host.recentChange == (modules.ConsensusChangeID{}) || ht.host.blockHeight == 0 {
		t.Fatal("host variables do not indicate that the host is tracking the consensus set correctly")
	}
	oldChange := ht.host.recentChange
	oldHeight := ht.host.blockHeight

	// Corrupt the variables and perform a rescan to see if they reset
	// correctly.
	ht.host.recentChange[0]++
	ht.host.blockHeight += 100e3
	ht.cs.Unsubscribe(ht.host)
	err = ht.host.startupRescan()
	if err != nil {
		t.Fatal(err)
	}
	if oldChange != ht.host.recentChange || oldHeight != ht.host.blockHeight {
		t.Error("consensus tracking variables were not reset correctly after rescan")
	}
}

// TestIntegrationAutoRescan checks that a rescan is triggered during New if
// the consensus set becomes desynchronized.
func TestIntegrationAutoRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestIntegrationAutoRescan")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the host's persistent variables have incorporated the first
	// few blocks.
	if ht.host.recentChange == (modules.ConsensusChangeID{}) || ht.host.blockHeight == 0 {
		t.Fatal("host variables do not indicate that the host is tracking the consensus set correctly")
	}
	oldChange := ht.host.recentChange
	oldHeight := ht.host.blockHeight

	// Corrupt the variables, then close the host.
	ht.host.recentChange[0]++
	ht.host.blockHeight += 100e3
	err = ht.host.Close() // host saves upon closing
	if err != nil {
		t.Fatal(err)
	}

	// Create a new host and check that the persist variables have correctly
	// reset.
	h, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	if oldChange != h.recentChange || oldHeight != h.blockHeight {
		t.Error("consensus tracking variables were not reset correctly after rescan")
	}
}

// TestSetAndGetSettings checks that the functions for interacting with the
// hosts settings object are working as expected.
func TestSetAndGetSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestSetAndGetSettings")
	if err != nil {
		t.Fatal(err)
	}

	// Check the default settings get returned at first call.
	settings := ht.host.Settings()
	if settings.TotalStorage != defaultTotalStorage {
		t.Error("settings GET did not return default value")
	}
	if settings.MaxDuration != defaultMaxDuration {
		t.Error("settings GET did not return default value")
	}
	if settings.WindowSize != defaultWindowSize {
		t.Error("settings GET did not return default value")
	}
	if settings.Price.Cmp(defaultPrice) != 0 {
		t.Error("settings GET did not return default value")
	}
	if settings.Collateral.Cmp(defaultCollateral) != 0 {
		t.Error("settings GET did not return default value")
	}

	// Submit updated settings and check that the changes stuck.
	settings.TotalStorage += 15
	settings.MaxDuration += 16
	settings.WindowSize += 17
	settings.Price = settings.Price.Add(types.NewCurrency64(18))
	settings.Collateral = settings.Collateral.Add(types.NewCurrency64(19))
	err = ht.host.SetSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	newSettings := ht.host.Settings()
	if settings.TotalStorage != newSettings.TotalStorage {
		t.Error("settings GET did not return updated value")
	}
	if settings.MaxDuration != newSettings.MaxDuration {
		t.Error("settings GET did not return updated value")
	}
	if settings.WindowSize != newSettings.WindowSize {
		t.Error("settings GET did not return updated value")
	}
	if settings.Price.Cmp(newSettings.Price) != 0 {
		t.Error("settings GET did not return updated value")
	}
	if settings.Collateral.Cmp(newSettings.Collateral) != 0 {
		t.Error("settings GET did not return updated value")
	}
}

// TestSetUnlockHash tries setting the unlock hash using SetSettings, an error
// should be returned.
func TestSetUnlockHash(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestSetAndGetSettings")
	if err != nil {
		t.Fatal(err)
	}

	// Get the settings and try changing the unlock hash.
	settings := ht.host.Settings()
	settings.UnlockHash[0]++
	err = ht.host.SetSettings(settings)
	if err != errChangedUnlockHash {
		t.Error("unlock hash was changed by SetSettings")
	}
}

// TestPersistentSettings checks that settings persist between instances of the
// host.
func TestPersistentSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestSetAndGetSettings")
	if err != nil {
		t.Fatal(err)
	}

	// Submit updated settings.
	settings := ht.host.Settings()
	settings.TotalStorage += 25
	settings.MaxDuration += 36
	settings.WindowSize += 47
	settings.Price = settings.Price.Add(types.NewCurrency64(38))
	settings.Collateral = settings.Collateral.Add(types.NewCurrency64(99))
	err = ht.host.SetSettings(settings)
	if err != nil {
		t.Fatal(err)
	}

	// Reboot the host and verify that the new settings stuck.
	err = ht.host.Close() // host saves upon closing
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	newSettings := h.Settings()
	if settings.TotalStorage != newSettings.TotalStorage {
		t.Error("settings GET did not return updated value:", settings.TotalStorage, "vs", newSettings.TotalStorage)
	}
	if settings.MaxDuration != newSettings.MaxDuration {
		t.Error("settings GET did not return updated value")
	}
	if settings.WindowSize != newSettings.WindowSize {
		t.Error("settings GET did not return updated value")
	}
	if settings.Price.Cmp(newSettings.Price) != 0 {
		t.Error("settings GET did not return updated value")
	}
	if settings.Collateral.Cmp(newSettings.Collateral) != 0 {
		t.Error("settings GET did not return updated value")
	}
}
