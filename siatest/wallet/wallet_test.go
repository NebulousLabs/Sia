package wallet

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/siatest"
	"github.com/NebulousLabs/Sia/types"
)

// TestTransactionReorg makes sure that a processedTransaction isn't returned
// by the API after bein reverted.
func TestTransactionReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	testdir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create two miners
	miner1, err := siatest.NewNode(siatest.Miner(filepath.Join(testdir, "miner1")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := miner1.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	// miner1 sends a txn to itself and mines it.
	uc, err := miner1.WalletAddressGet()
	if err != nil {
		t.Fatal(err)
	}
	wsp, err := miner1.WalletSiacoinsPost(types.SiacoinPrecision, uc.Address)
	if err != nil {
		t.Fatal(err)
	}
	blocks := 1
	for i := 0; i < blocks; i++ {
		if err := miner1.MineBlock(); err != nil {
			t.Fatal(err)
		}
	}
	// wait until the transaction from before shows up as processed.
	txn := wsp.TransactionIDs[len(wsp.TransactionIDs)-1]
	err = build.Retry(100, 100*time.Millisecond, func() error {
		cg, err := miner1.ConsensusGet()
		if err != nil {
			return err
		}
		wtg, err := miner1.WalletTransactionsGet(1, cg.Height)
		if err != nil {
			return err
		}
		for _, t := range wtg.ConfirmedTransactions {
			if t.TransactionID == txn {
				return nil
			}
		}
		return errors.New("txn isn't processed yet")
	})
	if err != nil {
		t.Fatal(err)
	}
	miner2, err := siatest.NewNode(siatest.Miner(filepath.Join(testdir, "miner2")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := miner2.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// miner2 mines 2 blocks now to create a longer chain than miner1.
	for i := 0; i < blocks+1; i++ {
		if err := miner2.MineBlock(); err != nil {
			t.Fatal(err)
		}
	}
	// miner1 and miner2 connect. This should cause a reorg that reverts the
	// transaction from before.
	if err := miner1.GatewayConnectPost(miner2.GatewayAddress()); err != nil {
		t.Fatal(err)
	}
	err = build.Retry(100, 100*time.Millisecond, func() error {
		cg, err := miner1.ConsensusGet()
		if err != nil {
			return err
		}
		wtg, err := miner1.WalletTransactionsGet(1, cg.Height)
		if err != nil {
			return err
		}
		for _, t := range wtg.ConfirmedTransactions {
			if t.TransactionID == txn {
				return errors.New("txn is still processed")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
