package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// TestScanLargeIndex tests the limits of the seedScanner.scan function.
func TestScanLargeIndex(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// create an empty wallet
	wt, err := createBlankWalletTester("TestScanLargeIndex")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	var masterKey crypto.TwofishKey
	fastrand.Read(masterKey[:])
	_, err = wt.wallet.Encrypt(masterKey)
	if err != nil {
		t.Fatal(err)
	}
	err = wt.wallet.Unlock(masterKey)
	if err != nil {
		t.Fatal(err)
	}

	// set the wallet's seed progress to a high number and then mine some coins.
	wt.wallet.mu.Lock()
	dbPutPrimarySeedProgress(wt.wallet.dbTx, numInitialKeys+1)
	wt.wallet.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}

	}

	// send money to ourselves so that we sweep a real output (instead of just
	// a miner payout)
	uc, err := wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// create seed scanner and scan the block
	seed, _, _ := wt.wallet.PrimarySeed()
	ss := newSeedScanner(seed, wt.wallet.log)
	err = ss.scan(wt.cs, wt.wallet.tg.StopChan())
	if err != nil {
		t.Fatal(err)
	}

	// no outputs should have been added
	if len(ss.siacoinOutputs) != 0 {
		t.Error("expected 0 outputs, got", len(ss.siacoinOutputs))
		for _, o := range ss.siacoinOutputs {
			t.Log(o.seedIndex, o.value)
		}
	}
	if ss.largestIndexSeen != 0 {
		t.Error("expected no index to be seen, got", ss.largestIndexSeen)
	}
}

// TestScanLoop tests that the scan loop will continue to run as long as it
// finds indices in the upper half of the last set of generated keys.
func TestScanLoop(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}

	// create a wallet
	wt, err := createWalletTester("TestScanLoop")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// send money to ourselves at four specific indices. This should cause the
	// scanner to loop exactly three times.
	indices := []uint64{500, 2500, 8000, 100000}
	for _, index := range indices {
		wt.wallet.mu.Lock()
		dbPutPrimarySeedProgress(wt.wallet.dbTx, index)
		wt.wallet.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}

		uc, err := wt.wallet.NextAddress()
		if err != nil {
			t.Fatal(err)
		}
		_, err = wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// create seed scanner and scan the block
	seed, _, _ := wt.wallet.PrimarySeed()
	ss := newSeedScanner(seed, wt.wallet.log)
	err = ss.scan(wt.cs, wt.wallet.tg.StopChan())
	if err != nil {
		t.Fatal(err)
	}

	// the scanner should have generated a specific number of keys
	expected := numInitialKeys + (numInitialKeys * scanMultiplier) + (numInitialKeys * scanMultiplier * scanMultiplier)
	if uint64(len(ss.keys)) != expected {
		t.Errorf("expected %v keys, got %v", expected, len(ss.keys))
	}
	// the largest index seen should be the penultimate element (+2, since 2
	// addresses are generated when sending coins). The last element should
	// not be seen, because it was outside the scanning range.
	if ss.largestIndexSeen != indices[len(indices)-2]+2 {
		t.Errorf("expected largest index to be %v, got %v", indices[len(indices)-2]+2, ss.largestIndexSeen)
	}
}
