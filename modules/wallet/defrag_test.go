package wallet

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestDefragWallet mines many blocks and checks that the wallet's outputs are
// consolidated once more than defragThreshold blocks are mined.
func TestDefragWallet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// mine defragThreshold blocks, resulting in defragThreshold outputs
	for i := 0; i < defragThreshold; i++ {
		_, err := wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// add another block to push the number of outputs over the threshold
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// allow some time for the defrag transaction to occur, then mine another block
	time.Sleep(time.Second * 5)

	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// defrag should keep the outputs below the threshold
	wt.wallet.mu.Lock()
	// force a sync because bucket stats may not be reliable until commit
	wt.wallet.syncDB()
	siacoinOutputs := wt.wallet.dbTx.Bucket(bucketSiacoinOutputs).Stats().KeyN
	wt.wallet.mu.Unlock()
	if siacoinOutputs > defragThreshold {
		t.Fatalf("defrag should result in fewer than defragThreshold outputs, got %v wanted %v\n", siacoinOutputs, defragThreshold)
	}
}

// TestDefragWalletDust verifies that dust outputs do not trigger the defrag
// operation.
func TestDefragWalletDust(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	dustOutputValue := types.NewCurrency64(10000)
	noutputs := defragThreshold + 1

	tbuilder, err := wt.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	err = tbuilder.FundSiacoins(dustOutputValue.Mul64(uint64(noutputs)))
	if err != nil {
		t.Fatal(err)
	}

	wt.wallet.mu.Lock()
	var dest types.UnlockHash
	for k := range wt.wallet.keys {
		dest = k
		break
	}
	wt.wallet.mu.Unlock()

	for i := 0; i < noutputs; i++ {
		tbuilder.AddSiacoinOutput(types.SiacoinOutput{
			Value:      dustOutputValue,
			UnlockHash: dest,
		})
	}

	txns, err := tbuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	err = wt.tpool.AcceptTransactionSet(txns)
	if err != nil {
		t.Fatal(err)
	}

	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	wt.wallet.mu.Lock()
	// force a sync because bucket stats may not be reliable until commit
	wt.wallet.syncDB()
	siacoinOutputs := wt.wallet.dbTx.Bucket(bucketSiacoinOutputs).Stats().KeyN
	wt.wallet.mu.Unlock()
	if siacoinOutputs < defragThreshold {
		t.Fatal("defrag consolidated dust outputs")
	}
}

// TestDefragOutputExhaustion verifies that sending transactions still succeeds
// even when the defragger is under heavy stress.
func TestDefragOutputExhaustion(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	wt.wallet.mu.Lock()
	var dest types.UnlockHash
	for k := range wt.wallet.keys {
		dest = k
		break
	}
	wt.wallet.mu.Unlock()

	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// concurrently make a bunch of transactions with lots of outputs to keep the
	// defragger running
	closechan := make(chan struct{})
	donechan := make(chan struct{})
	go func() {
		defer close(donechan)
		for {
			select {
			case <-closechan:
				return
			case <-time.After(time.Millisecond * 100):
				_, err := wt.miner.AddBlock()
				if err != nil {
					t.Fatal(err)
				}
				txnValue := types.SiacoinPrecision.Mul64(3000)
				fee := types.SiacoinPrecision.Mul64(10)
				numOutputs := defragThreshold + 1

				tbuilder, err := wt.wallet.StartTransaction()
				if err != nil {
					t.Fatal(err)
				}

				tbuilder.FundSiacoins(txnValue.Mul64(uint64(numOutputs)).Add(fee))

				for i := 0; i < numOutputs; i++ {
					tbuilder.AddSiacoinOutput(types.SiacoinOutput{
						Value:      txnValue,
						UnlockHash: dest,
					})
				}

				tbuilder.AddMinerFee(fee)

				txns, err := tbuilder.Sign(true)
				if err != nil {
					t.Error("Error signing fragmenting transaction:", err)
				}
				err = wt.tpool.AcceptTransactionSet(txns)
				if err != nil {
					t.Error("Error accepting fragmenting transaction:", err)
				}
				_, err = wt.miner.AddBlock()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}()

	time.Sleep(time.Second * 1)

	// ensure we can still send transactions while receiving aggressively
	// fragmented outputs
	for i := 0; i < 30; i++ {
		sendAmount := types.SiacoinPrecision.Mul64(2000)
		_, err = wt.wallet.SendSiacoins(sendAmount, types.UnlockHash{})
		if err != nil {
			t.Errorf("%v: %v", i, err)
		}
		time.Sleep(time.Millisecond * 50)
	}

	close(closechan)
	<-donechan
}

// TestDefragInterrupted checks that a failing defrag unmarks spent outputs correctly
func TestDefragInterrupted(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	deps := dependencyDefragInterrupted{}
	deps.fail()
	wt, err := createWalletTester(t.Name(), &deps)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// mine defragThreshold blocks, resulting in defragThreshold outputs
	for i := 0; i < defragThreshold; i++ {
		_, err := wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// allow some time for the defrag transaction to occur, then mine another block
	time.Sleep(time.Second * 5)

	wt.wallet.mu.Lock()
	// force a sync because bucket stats may not be reliable until commit
	wt.wallet.syncDB()
	spentOutputs := wt.wallet.dbTx.Bucket(bucketSpentOutputs).Stats().KeyN
	siacoinOutputs := wt.wallet.dbTx.Bucket(bucketSiacoinOutputs).Stats().KeyN
	wt.wallet.mu.Unlock()
	if siacoinOutputs <= defragThreshold {
		t.Fatal("not enough outputs created - defrag wasn't triggered")
	}
	if spentOutputs > 0 {
		t.Fatalf("There should be 0 outputs in the database since defrag failed but there were %v",
			spentOutputs)
	}

	// Trigger defrag again
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// allow some time for the defrag
	time.Sleep(time.Second * 5)

	// Mine another block to update the wallet
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Defrag should have worked this time
	wt.wallet.mu.Lock()
	// force a sync because bucket stats may not be reliable until commit
	wt.wallet.syncDB()
	spentOutputs = wt.wallet.dbTx.Bucket(bucketSpentOutputs).Stats().KeyN
	siacoinOutputs = wt.wallet.dbTx.Bucket(bucketSiacoinOutputs).Stats().KeyN
	wt.wallet.mu.Unlock()
	if siacoinOutputs > defragThreshold {
		t.Fatalf("defrag should result in fewer than defragThreshold outputs, got %v wanted %v\n", siacoinOutputs, defragThreshold)
	}
	if spentOutputs == 0 {
		t.Fatalf("There should be > 0 spentOutputs")
	}

}
