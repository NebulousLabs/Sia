package wallet
/*

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestSiagKeyLoading loads the types testing keys into memory and checks that
// the wallet correctly discovers them.
func TestSiagKeyLoading(t *testing.T) {
	wt, err := createWalletTester("TestSiagKeyLoading")
	if err != nil {
		t.Fatal(err)
	}

	// Check that 0 siafunds are recognized by the wallet.
	siafundBalance, siacoinClaimBalance := wt.wallet.SiafundBalance()
	if siafundBalance.Cmp(types.ZeroCurrency) != 0 || siacoinClaimBalance.Cmp(types.ZeroCurrency) != 0 {
		t.Fatal("Wallet did not start with empty siafund and siaclaim balances")
	}

	// Load the 1-of-1 key and see if it is recognized after restart.
	err = wt.wallet.WatchSiagSiafundAddress("../../types/siag0of1of1.siakey")
	if err != nil {
		t.Fatal(err)
	}
	// TODO: Need some way to rescan. Until then, manual testing will need to
	// suffice.
}
*/
