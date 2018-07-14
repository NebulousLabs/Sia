package contractor

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// newStub is used to test the New function. It implements all of the contractor's
// dependencies.
type newStub struct{}

// consensus set stubs
func (newStub) ConsensusSetSubscribe(modules.ConsensusSetSubscriber, modules.ConsensusChangeID, <-chan struct{}) error {
	return nil
}
func (newStub) Synced() bool                               { return true }
func (newStub) Unsubscribe(modules.ConsensusSetSubscriber) { return }

// wallet stubs
func (newStub) NextAddress() (uc types.UnlockConditions, err error)          { return }
func (newStub) StartTransaction() (tb modules.TransactionBuilder, err error) { return }

// transaction pool stubs
func (newStub) AcceptTransactionSet([]types.Transaction) error      { return nil }
func (newStub) FeeEstimation() (a types.Currency, b types.Currency) { return }

// hdb stubs
func (newStub) AllHosts() []modules.HostDBEntry                                      { return nil }
func (newStub) ActiveHosts() []modules.HostDBEntry                                   { return nil }
func (newStub) Host(types.SiaPublicKey) (settings modules.HostDBEntry, ok bool)      { return }
func (newStub) IncrementSuccessfulInteractions(key types.SiaPublicKey)               { return }
func (newStub) IncrementFailedInteractions(key types.SiaPublicKey)                   { return }
func (newStub) RandomHosts(int, []types.SiaPublicKey) ([]modules.HostDBEntry, error) { return nil, nil }
func (newStub) ScoreBreakdown(modules.HostDBEntry) modules.HostScoreBreakdown {
	return modules.HostScoreBreakdown{}
}

// TestNew tests the New function.
func TestNew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Using a stub implementation of the dependencies is fine, as long as its
	// non-nil.
	var stub newStub
	dir := build.TempDir("contractor", t.Name())

	// Sane values.
	_, err := New(stub, stub, stub, stub, dir)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Nil consensus set.
	_, err = New(nil, stub, stub, stub, dir)
	if err != errNilCS {
		t.Fatalf("expected %v, got %v", errNilCS, err)
	}

	// Nil wallet.
	_, err = New(stub, nil, stub, stub, dir)
	if err != errNilWallet {
		t.Fatalf("expected %v, got %v", errNilWallet, err)
	}

	// Nil transaction pool.
	_, err = New(stub, stub, nil, stub, dir)
	if err != errNilTpool {
		t.Fatalf("expected %v, got %v", errNilTpool, err)
	}

	// Bad persistDir.
	_, err = New(stub, stub, stub, stub, "")
	if !os.IsNotExist(err) {
		t.Fatalf("expected invalid directory, got %v", err)
	}
}

// TestAllowance tests the Allowance method.
func TestAllowance(t *testing.T) {
	c := &Contractor{
		allowance: modules.Allowance{
			Funds:  types.NewCurrency64(1),
			Period: 2,
			Hosts:  3,
		},
	}
	a := c.Allowance()
	if a.Funds.Cmp(c.allowance.Funds) != 0 ||
		a.Period != c.allowance.Period ||
		a.Hosts != c.allowance.Hosts {
		t.Fatal("Allowance did not return correct allowance:", a, c.allowance)
	}
}

// stubHostDB mocks the hostDB dependency using zero-valued implementations of
// its methods.
type stubHostDB struct{}

func (stubHostDB) AllHosts() (hs []modules.HostDBEntry)                                      { return }
func (stubHostDB) ActiveHosts() (hs []modules.HostDBEntry)                                   { return }
func (stubHostDB) Host(types.SiaPublicKey) (h modules.HostDBEntry, ok bool)                  { return }
func (stubHostDB) IncrementSuccessfulInteractions(key types.SiaPublicKey)                    { return }
func (stubHostDB) IncrementFailedInteractions(key types.SiaPublicKey)                        { return }
func (stubHostDB) PublicKey() (spk types.SiaPublicKey)                                       { return }
func (stubHostDB) RandomHosts(int, []types.SiaPublicKey) (hs []modules.HostDBEntry, _ error) { return }
func (stubHostDB) ScoreBreakdown(modules.HostDBEntry) modules.HostScoreBreakdown {
	return modules.HostScoreBreakdown{}
}

// TestAllowanceSpending verifies that the contractor will not spend more or
// less than the allowance if uploading causes repeated early renewal, and that
// correct spending metrics are returned, even across renewals.
func TestAllowanceSpending(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create testing trio
	h, c, m, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// make the host's upload price very high so this test requires less
	// computation
	settings := h.InternalSettings()
	settings.MinUploadBandwidthPrice = types.SiacoinPrecision.Div64(10)
	err = h.SetInternalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	err = h.Announce()
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		hosts, err := c.hdb.RandomHosts(1, nil)
		if err != nil {
			return err
		}
		if len(hosts) == 0 {
			return errors.New("host has not been scanned yet")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// set an allowance
	testAllowance := modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(6000),
		RenewWindow: 100,
		Hosts:       1,
		Period:      200,
	}
	err = c.SetAllowance(testAllowance)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 1 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	// exhaust a contract and add a block several times. Despite repeatedly
	// running out of funds, the contractor should not spend more than the
	// allowance.
	for i := 0; i < 15; i++ {
		for _, contract := range c.Contracts() {
			ed, err := c.Editor(contract.HostPublicKey, nil)
			if err != nil {
				continue
			}

			// upload 10 sectors to the contract
			for sec := 0; sec < 10; sec++ {
				ed.Upload(make([]byte, modules.SectorSize))
			}
			err = ed.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		_, err := m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	var minerRewards types.Currency
	w := c.wallet.(*WalletBridge).W.(modules.Wallet)
	txns, err := w.Transactions(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, txn := range txns {
		for _, so := range txn.Outputs {
			if so.FundType == types.SpecifierMinerPayout {
				minerRewards = minerRewards.Add(so.Value)
			}
		}
	}
	balance, _, _, err := w.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	spent := minerRewards.Sub(balance)
	if spent.Cmp(testAllowance.Funds) > 0 {
		t.Fatal("contractor spent too much money: spent", spent.HumanString(), "allowance funds:", testAllowance.Funds.HumanString())
	}

	// we should have spent at least the allowance minus the cost of one more refresh
	refreshCost := c.Contracts()[0].TotalCost.Mul64(2)
	expectedMinSpending := testAllowance.Funds.Sub(refreshCost)
	if spent.Cmp(expectedMinSpending) < 0 {
		t.Fatal("contractor spent to little money: spent", spent.HumanString(), "expected at least:", expectedMinSpending.HumanString())
	}

	// PeriodSpending should reflect the amount of spending accurately
	reportedSpending := c.PeriodSpending()
	if reportedSpending.TotalAllocated.Cmp(spent) != 0 {
		t.Fatal("reported incorrect spending for this billing cycle: got", reportedSpending.TotalAllocated.HumanString(), "wanted", spent.HumanString())
	}
	// COMPATv132 totalallocated should equal contractspending field.
	if reportedSpending.ContractSpendingDeprecated.Cmp(reportedSpending.TotalAllocated) != 0 {
		t.Fatal("TotalAllocated should be equal to ContractSpending for compatibility")
	}

	var expectedFees types.Currency
	for _, contract := range c.Contracts() {
		expectedFees = expectedFees.Add(contract.TxnFee)
		expectedFees = expectedFees.Add(contract.SiafundFee)
		expectedFees = expectedFees.Add(contract.ContractFee)
	}
	if expectedFees.Cmp(reportedSpending.ContractFees) != 0 {
		t.Fatalf("expected %v reported fees but was %v",
			expectedFees.HumanString(), reportedSpending.ContractFees.HumanString())
	}
}

// TestIntegrationSetAllowance tests the SetAllowance method.
func TestIntegrationSetAllowance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create testing trio
	_, c, m, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// this test requires two hosts: create another one
	h, err := newTestingHost(build.TempDir("hostdata", ""), c.cs.(modules.ConsensusSet), c.tpool.(modules.TransactionPool))
	if err != nil {
		t.Fatal(err)
	}

	// announce the extra host
	err = h.Announce()
	if err != nil {
		t.Fatal(err)
	}

	// mine a block, processing the announcement
	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// wait for hostdb to scan
	hosts, err := c.hdb.RandomHosts(1, nil)
	if err != nil {
		t.Fatal("failed to get hosts", err)
	}
	for i := 0; i < 100 && len(hosts) == 0; i++ {
		time.Sleep(time.Millisecond * 50)
	}

	// cancel allowance
	var a modules.Allowance
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}

	// bad args
	a.Hosts = 1
	err = c.SetAllowance(a)
	if err != errAllowanceZeroPeriod {
		t.Errorf("expected %q, got %q", errAllowanceZeroPeriod, err)
	}
	a.Period = 20
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroWindow {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroWindow, err)
	}
	a.RenewWindow = 20
	err = c.SetAllowance(a)
	if err != errAllowanceWindowSize {
		t.Errorf("expected %q, got %q", errAllowanceWindowSize, err)
	}

	// reasonable values; should succeed
	a.Funds = types.SiacoinPrecision.Mul64(100)
	a.RenewWindow = 10
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 1 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	// set same allowance; should no-op
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	clen := c.staticContracts.Len()
	if clen != 1 {
		t.Fatal("expected 1 contract, got", clen)
	}

	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// set allowance with Hosts = 2; should only form one new contract
	a.Hosts = 2
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 2 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// set allowance with Funds*2; should trigger renewal of both contracts
	a.Funds = a.Funds.Mul64(2)
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 2 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	// delete one of the contracts and set allowance with Funds*2; should
	// trigger 1 renewal and 1 new contract
	c.mu.Lock()
	ids := c.staticContracts.IDs()
	contract, _ := c.staticContracts.Acquire(ids[0])
	c.staticContracts.Delete(contract)
	c.mu.Unlock()
	a.Funds = a.Funds.Mul64(2)
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 2 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// testWalletShim is used to test the walletBridge type.
type testWalletShim struct {
	nextAddressCalled bool
	startTxnCalled    bool
}

// These stub implementations for the walletShim interface set their respective
// booleans to true, allowing tests to verify that they have been called.
func (ws *testWalletShim) NextAddress() (types.UnlockConditions, error) {
	ws.nextAddressCalled = true
	return types.UnlockConditions{}, nil
}
func (ws *testWalletShim) StartTransaction() (modules.TransactionBuilder, error) {
	ws.startTxnCalled = true
	return nil, nil
}

// TestWalletBridge tests the walletBridge type.
func TestWalletBridge(t *testing.T) {
	shim := new(testWalletShim)
	bridge := WalletBridge{shim}
	bridge.NextAddress()
	if !shim.nextAddressCalled {
		t.Error("NextAddress was not called on the shim")
	}
	bridge.StartTransaction()
	if !shim.startTxnCalled {
		t.Error("StartTransaction was not called on the shim")
	}
}
