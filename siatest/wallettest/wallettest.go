// Package wallettest contains integration tests that are specific to the
// wallet.
package wallettest

import (
	"testing"

	"github.com/NebulousLabs/Sia/siatest"
)

// TestWalletQueries establishes a wallet of progressively increasing
// complexity, and verifies that it can reasonably tolerate a variety of
// queries.
func TestWalletQueries(t *testing.T) {
	testDir, err := siatest.TestDir("wallettest", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	tn, err := NewTestNode(NewTestNodeParams{Dir: testDir})
	if err != nil {
		t.Fatal(err)
	}
}
