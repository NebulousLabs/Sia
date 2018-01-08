package wallet

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/types"
)

// TestRebroadcastTransactions checks if transactions are correctly
// rebroadcasted after some time if they haven't been confirmed
func TestRebroadcastTransactions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), &ProductionDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Get an address to send money to
	uc, err := wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	// Send money to the address
	_, err = wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
	if err != nil {
		t.Fatal(err)
	}
	// The wallet should track the new tSet
	if len(wt.wallet.broadcastedTSets) != 1 {
		t.Fatalf("len(broadcastedTSets) should be %v but was %v",
			1, len(wt.wallet.broadcastedTSets))
	}
	// Mine enough blocks for the wallet to stop tracking the tSet
	for i := 0; i < RebroadcastInterval+1; i++ {
		if _, err := wt.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}
	if len(wt.wallet.broadcastedTSets) > 0 {
		t.Fatalf("len(broadcastedTSets) should be 0 but was %v",
			len(wt.wallet.broadcastedTSets))
	}

	// Send some more money to the address
	tSet, err := wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
	if err != nil {
		t.Fatal(err)
	}
	// The wallet should track the new tSet
	if len(wt.wallet.broadcastedTSets) != 1 {
		t.Fatalf("len(broadcastedTSets) should be %v but was %v",
			1, len(wt.wallet.broadcastedTSets))
	}
	// Mine a block to get the tSet confirmed
	if _, err := wt.miner.AddBlock(); err != nil {
		t.Fatal(err)
	}
	// Corrupt the new tSet to make sure the wallet believes it is not confirmed
	tSetID := modules.TransactionSetID(crypto.HashAll(tSet))
	bts := wt.wallet.broadcastedTSets[tSetID]
	for tid := range bts.confirmedTxn {
		bts.confirmedTxn[tid] = false
	}
	// Mine the same number of blocks. This time the wallet should still track
	// the tSet afterwards.
	for i := 0; i < RebroadcastInterval; i++ {
		if _, err := wt.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}
	if len(wt.wallet.broadcastedTSets) != 1 {
		t.Fatalf("The wallet should still track the tSet")
	}
	// Continue mining to make sure that the wallet stops tracking the tSet
	// once the max number of retries is reached
	for i := types.BlockHeight(0); i < RebroadcastTimeout; i++ {
		if _, err := wt.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}
	if _, exists := wt.wallet.broadcastedTSets[tSetID]; exists {
		t.Fatalf("Wallet should drop txnSet after %v blocks", RebroadcastTimeout)
	}
}

// TestRebroadcastTransactionsPersist checks if the wallet keeps tracking
// transactions for rebroadcasting after a restart
func TestRebroadcastTransactionsPersist(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), &ProductionDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Get an address to send money to
	uc, err := wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	// Send money to the address
	tSet, err := wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
	if err != nil {
		t.Fatal(err)
	}
	// The wallet should track the tSet
	if len(wt.wallet.broadcastedTSets) != 1 {
		t.Fatalf("len(broadcastedTSets) should be %v but was %v",
			1, len(wt.wallet.broadcastedTSets))
	}
	// Mine a block to get the tSet confirmed
	if _, err := wt.miner.AddBlock(); err != nil {
		t.Fatal(err)
	}
	// Corrupt the tSet to make sure the wallet believes it is not confirmed
	tSetID := modules.TransactionSetID(crypto.HashAll(tSet))
	bts := wt.wallet.broadcastedTSets[tSetID]
	bts.confirmedTxn[tSet[0].ID()] = false
	if err := bts.confirmed(tSet[0].ID(), false); err != nil {
		t.Fatal(err)
	}

	// Close and restart the wallet and miner
	if err := wt.wallet.Close(); err != nil {
		t.Fatal(err)
	}
	if err := wt.miner.Close(); err != nil {
		t.Fatal(err)
	}
	wt.wallet, err = New(wt.cs, wt.tpool, filepath.Join(wt.persistDir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.wallet.Unlock(wt.walletMasterKey); err != nil {
		t.Fatal(err)
	}
	wt.miner, err = miner.New(wt.cs, wt.tpool, wt.wallet, filepath.Join(wt.persistDir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	// The wallet should still track the new tSet
	if len(wt.wallet.broadcastedTSets) != 1 {
		t.Fatalf("len(broadcastedTSets) should be %v but was %v",
			1, len(wt.wallet.broadcastedTSets))
	}
	// The same transactions should be marked as confirmed
	btsNew := wt.wallet.broadcastedTSets[tSetID]
	for key := range btsNew.confirmedTxn {
		if btsNew.confirmedTxn[key] != bts.confirmedTxn[key] {
			t.Fatalf("txn confirmation state should be %v but was %v",
				bts.confirmedTxn[key], btsNew.confirmedTxn[key])
		}
	}
	// Mine rebroadcastInterval blocks. The wallet should keep tracking the
	// tSet afterwards
	for i := 0; i < RebroadcastInterval; i++ {
		if _, err := wt.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}
	if len(wt.wallet.broadcastedTSets) != 1 {
		t.Fatalf("The wallet should still track the tSet")
	}
	// Continue mining to make sure that the wallet stops tracking the tSet
	// once the max number of retries is reached
	for i := types.BlockHeight(0); i < RebroadcastTimeout; i++ {
		if _, err := wt.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}
	if _, exists := wt.wallet.broadcastedTSets[tSetID]; exists {
		t.Fatalf("Wallet should drop txnSet after %v blocks", RebroadcastTimeout)
	}
}
