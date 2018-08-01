package wallet

import (
	"sort"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestSendSiacoins probes the SendSiacoins method of the wallet.
func TestSendSiacoins(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Get the initial balance - should be 1 block. The unconfirmed balances
	// should be 0.
	confirmedBal, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	unconfirmedOut, unconfirmedIn, err := wt.wallet.UnconfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	if !confirmedBal.Equals(types.CalculateCoinbase(1)) {
		t.Error("unexpected confirmed balance")
	}
	if !unconfirmedOut.Equals(types.ZeroCurrency) {
		t.Error("unconfirmed balance should be 0")
	}
	if !unconfirmedIn.Equals(types.ZeroCurrency) {
		t.Error("unconfirmed balance should be 0")
	}

	// Send 5000 hastings. The wallet will automatically add a fee. Outgoing
	// unconfirmed siacoins - incoming unconfirmed siacoins should equal 5000 +
	// fee.
	sendValue := types.SiacoinPrecision.Mul64(3)
	_, tpoolFee := wt.wallet.tpool.FeeEstimation()
	tpoolFee = tpoolFee.Mul64(750)
	_, err = wt.wallet.SendSiacoins(sendValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	confirmedBal2, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	unconfirmedOut2, unconfirmedIn2, err := wt.wallet.UnconfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	if !confirmedBal2.Equals(confirmedBal) {
		t.Error("confirmed balance changed without introduction of blocks")
	}
	if !unconfirmedOut2.Equals(unconfirmedIn2.Add(sendValue).Add(tpoolFee)) {
		t.Error("sending siacoins appears to be ineffective")
	}

	// Move the balance into the confirmed set.
	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	confirmedBal3, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	unconfirmedOut3, unconfirmedIn3, err := wt.wallet.UnconfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	if !confirmedBal3.Equals(confirmedBal2.Add(types.CalculateCoinbase(2)).Sub(sendValue).Sub(tpoolFee)) {
		t.Error("confirmed balance did not adjust to the expected value")
	}
	if !unconfirmedOut3.Equals(types.ZeroCurrency) {
		t.Error("unconfirmed balance should be 0")
	}
	if !unconfirmedIn3.Equals(types.ZeroCurrency) {
		t.Error("unconfirmed balance should be 0")
	}
}

// TestIntegrationSendOverUnder sends too many siacoins, resulting in an error,
// followed by sending few enough siacoins that the send should complete.
//
// This test is here because of a bug found in production where the wallet
// would mark outputs as spent before it knew that there was enough money  to
// complete the transaction. This meant that, after trying to send too many
// coins, all outputs got marked 'sent'. This test reproduces those conditions
// to ensure it does not happen again.
func TestIntegrationSendOverUnder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Spend too many siacoins.
	tooManyCoins := types.SiacoinPrecision.Mul64(1e12)
	_, err = wt.wallet.SendSiacoins(tooManyCoins, types.UnlockHash{})
	if err == nil {
		t.Error("low balance err not returned after attempting to send too many coins:", err)
	}

	// Spend a reasonable amount of siacoins.
	reasonableCoins := types.SiacoinPrecision.Mul64(100e3)
	_, err = wt.wallet.SendSiacoins(reasonableCoins, types.UnlockHash{})
	if err != nil {
		t.Error("unexpected error: ", err)
	}
}

// TestIntegrationSpendHalfHalf spends more than half of the coins, and then
// more than half of the coins again, to make sure that the wallet is not
// reusing outputs that it has already spent.
func TestIntegrationSpendHalfHalf(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Spend more than half of the coins twice.
	halfPlus := types.SiacoinPrecision.Mul64(200e3)
	_, err = wt.wallet.SendSiacoins(halfPlus, types.UnlockHash{})
	if err != nil {
		t.Error("unexpected error: ", err)
	}
	_, err = wt.wallet.SendSiacoins(halfPlus, types.UnlockHash{1})
	if err == nil {
		t.Error("wallet appears to be reusing outputs when building transactions: ", err)
	}
}

// TestIntegrationSpendUnconfirmed spends an unconfirmed siacoin output.
func TestIntegrationSpendUnconfirmed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Spend the only output.
	halfPlus := types.SiacoinPrecision.Mul64(200e3)
	_, err = wt.wallet.SendSiacoins(halfPlus, types.UnlockHash{})
	if err != nil {
		t.Error("unexpected error: ", err)
	}
	someMore := types.SiacoinPrecision.Mul64(75e3)
	_, err = wt.wallet.SendSiacoins(someMore, types.UnlockHash{1})
	if err != nil {
		t.Error("wallet appears to be struggling to spend unconfirmed outputs")
	}
}

// TestIntegrationSortedOutputsSorting checks that the outputs are being correctly sorted
// by the currency value.
func TestIntegrationSortedOutputsSorting(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	so := sortedOutputs{
		ids: []types.SiacoinOutputID{{0}, {1}, {2}, {3}, {4}, {5}, {6}, {7}},
		outputs: []types.SiacoinOutput{
			{Value: types.NewCurrency64(2)},
			{Value: types.NewCurrency64(3)},
			{Value: types.NewCurrency64(4)},
			{Value: types.NewCurrency64(7)},
			{Value: types.NewCurrency64(6)},
			{Value: types.NewCurrency64(0)},
			{Value: types.NewCurrency64(1)},
			{Value: types.NewCurrency64(5)},
		},
	}
	sort.Sort(so)

	expectedIDSorting := []types.SiacoinOutputID{{5}, {6}, {0}, {1}, {2}, {7}, {4}, {3}}
	for i := uint64(0); i < 8; i++ {
		if so.ids[i] != expectedIDSorting[i] {
			t.Error("an id is out of place: ", i)
		}
		if !so.outputs[i].Value.Equals64(i) {
			t.Error("a value is out of place: ", i)
		}
	}
}

// TestSendSiacoinsFailed checks if SendSiacoins and SendSiacoinsMulti behave
// correctly when funcing the Transaction succeeded but accepting it didn't.
func TestSendSiacoinsAcceptTxnSetFailed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	deps := &dependencySendSiacoinsInterrupted{}
	wt, err := createWalletTester(t.Name(), deps)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// There should be no spent transactions in the database at this point
	wt.wallet.mu.Lock()
	wt.wallet.syncDB()
	if wt.wallet.dbTx.Bucket(bucketSpentOutputs).Stats().KeyN != 0 {
		wt.wallet.mu.Unlock()
		t.Fatal("bucketSpentOutputs isn't empty")
	}
	wt.wallet.mu.Unlock()

	// Try to send coins using SendSiacoinsMulti
	numOutputs := 10
	scos := make([]types.SiacoinOutput, numOutputs)
	for i := 0; i < numOutputs; i++ {
		uc, err := wt.wallet.NextAddress()
		if err != nil {
			t.Fatal(err)
		}
		scos[i].Value = types.SiacoinPrecision
		scos[i].UnlockHash = uc.UnlockHash()
	}
	deps.fail()
	_, err = wt.wallet.SendSiacoinsMulti(scos)
	if err == nil {
		t.Fatal("SendSiacoinsMulti should have failed but didn't")
	}

	// Send some coins using SendSiacoins
	uc, err := wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	deps.fail()
	_, err = wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
	if err == nil {
		t.Fatal("SendSiacoins should have failed but didn't")
	}

	// There should still be no spent transactions in the database
	wt.wallet.mu.Lock()
	wt.wallet.syncDB()
	bucket := wt.wallet.dbTx.Bucket(bucketSpentOutputs)
	if bucket.Stats().KeyN != 0 {
		wt.wallet.mu.Unlock()
		t.Fatal("bucketSpentOutputs isn't empty")
	}
	wt.wallet.mu.Unlock()

	// Send the money again without the failing dependency
	_, err = wt.wallet.SendSiacoinsMulti(scos)
	if err != nil {
		t.Fatalf("SendSiacoinsMulti failed: %v", err)
	}

	// Send some coins using SendSiacoins
	_, err = wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
	if err != nil {
		t.Fatalf("SendSiacoins failed: %v", err)
	}
}
