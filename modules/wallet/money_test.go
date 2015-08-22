package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestSendSiacoins probes the SendSiacoins method of the wallet.
func TestSendSiacoins(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester("TestSendSiacoins")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Get the initial balance - should be 1 block. The unconfirmed balances
	// should be 0.
	confirmedBal, _, _ := wt.wallet.ConfirmedBalance()
	unconfirmedOut, unconfirmedIn := wt.wallet.UnconfirmedBalance()
	if confirmedBal.Cmp(types.CalculateCoinbase(1)) != 0 {
		t.Error("unexpected confirmed balance")
	}
	if unconfirmedOut.Cmp(types.ZeroCurrency) != 0 {
		t.Error("unconfirmed balance should be 0")
	}
	if unconfirmedIn.Cmp(types.ZeroCurrency) != 0 {
		t.Error("unconfirmed balance should be 0")
	}

	// Send 5000 hastings. The wallet will automatically add a fee. Outgoing
	// unconfirmed siacoins - incoming unconfirmed siacoins should equal 5000 +
	// fee.
	tpoolFee := types.NewCurrency64(10).Mul(types.SiacoinPrecision)
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5000), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	confirmedBal2, _, _ := wt.wallet.ConfirmedBalance()
	unconfirmedOut2, unconfirmedIn2 := wt.wallet.UnconfirmedBalance()
	if confirmedBal2.Cmp(confirmedBal) != 0 {
		t.Error("confirmed balance changed without introduction of blocks")
	}
	if unconfirmedOut2.Cmp(unconfirmedIn2.Add(types.NewCurrency64(5000)).Add(tpoolFee)) != 0 {
		t.Error("sending siacoins appears to be ineffective")
	}

	// Move the balance into the confirmed set.
	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	confirmedBal3, _, _ := wt.wallet.ConfirmedBalance()
	unconfirmedOut3, unconfirmedIn3 := wt.wallet.UnconfirmedBalance()
	if confirmedBal3.Cmp(confirmedBal2.Add(types.CalculateCoinbase(2)).Sub(types.NewCurrency64(5000)).Sub(tpoolFee)) != 0 {
		t.Error("confirmed balance did not adjust to the expected value")
	}
	if unconfirmedOut3.Cmp(types.ZeroCurrency) != 0 {
		t.Error("unconfirmed balance should be 0")
	}
	if unconfirmedIn3.Cmp(types.ZeroCurrency) != 0 {
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
	wt, err := createWalletTester("TestIntegrationSendOverUnder")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Send too many siacoins.
	tooManyCoins := types.SiacoinPrecision.Mul(types.NewCurrency64(1e12))
	_, err = wt.wallet.SendSiacoins(tooManyCoins, types.UnlockHash{})
	if err != modules.ErrLowBalance {
		t.Error("low balance err not returned after attempting to send too many coins")
	}

	// Send a reasonable amount of siacoins.
	reasonableCoins := types.SiacoinPrecision.Mul(types.NewCurrency64(100e3))
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
	wt, err := createWalletTester("TestIntegrationSendOverUnder")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Send more than half of the coins twice.
	halfPlus := types.SiacoinPrecision.Mul(types.NewCurrency64(200e3))
	_, err = wt.wallet.SendSiacoins(halfPlus, types.UnlockHash{})
	if err != nil {
		t.Error("unexpected error: ", err)
	}
	_, err = wt.wallet.SendSiacoins(halfPlus, types.UnlockHash{1})
	if err != modules.ErrPotentialDoubleSpend {
		t.Error("wallet appears to be reusing outputs when building transactions: ", err)
	}
}
