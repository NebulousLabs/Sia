package api

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// TestTransactionPoolRawHandler verifies that the transaction pools' raw
// transaction post endpoint works correctly.
func TestTransactionPoolRawHandlerPOST(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Spin up a second and fourth server tester, and get them all on the same
	// block. The fourth server tester will be used later, after a third is
	// created and used.
	st2, err := blankServerTester(t.Name() + "-st2")
	if err != nil {
		t.Fatal(err)
	}
	st4, err := blankServerTester(t.Name() + "-st4")
	if err != nil {
		t.Fatal(err)
	}
	err = fullyConnectNodes([]*serverTester{st, st2, st4})
	if err != nil {
		t.Fatal(err)
	}

	// Reset the peers, giving them different ip addresses, preventing them
	// from connecting to eachother.
	err = st.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = st2.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = st4.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	st, err = assembleServerTester(st.walletKey, st.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.panicClose()
	st2, err = assembleServerTester(st2.walletKey, st2.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.panicClose()
	st4, err = assembleServerTester(st4.walletKey, st4.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st4.panicClose()

	// Create a transaction on one node and fetch it.
	sentValue := types.SiacoinPrecision.Mul64(1000)
	txns, err := st.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	lastTxn := txns[len(txns)-1]
	var trg TpoolRawGET
	err = st.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the correctness of the transaction.
	var decodedTxn types.Transaction
	err = encoding.Unmarshal(trg.Transaction, &decodedTxn)
	if err != nil {
		t.Fatal(err)
	}
	if decodedTxn.ID() != lastTxn.ID() {
		t.Fatal("tpool raw get returned the wrong transaction")
	}
	// Verify the correctness of the parents.
	var decodedParents []types.Transaction
	err = encoding.Unmarshal(trg.Parents, &decodedParents)
	if err != nil {
		t.Fatal(err)
	}
	if len(decodedParents) != len(txns)-1 {
		t.Fatal("returned the incorrect number of parents")
	}

	// Transaction should not be visible on node 2.
	var trg2 TpoolRawGET
	err = st2.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg2)
	if err.Error() != "transaction not found in transaction pool" {
		t.Fatal("transaction should be missing initially from the second tpool")
	}

	// Try posting the transaction to node 2.
	postValues := url.Values{}
	postValues.Set("parents", string(trg.Parents))
	postValues.Set("transaction", string(trg.Transaction))
	err = st2.stdPostAPI("/tpool/raw", postValues)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the two transactions returned from each server are
	// identical.
	err = st2.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(trg2.Parents, trg.Parents) {
		t.Error("transaction parents mismatch")
	}
	if !bytes.Equal(trg2.Transaction, trg.Transaction) {
		t.Error("transaction parents mismatch")
	}

	// Create a third server tester, connect it to the second one.
	st3, err := blankServerTester(t.Name() + "-st3")
	if err != nil {
		t.Fatal(err)
	}
	defer st3.server.panicClose()
	err = fullyConnectNodes([]*serverTester{st2, st3})
	if err != nil {
		t.Fatal(err)
	}
	// Posting the raw transaction to the second server again should cause it
	// to be broadcast to the third server.
	err = st2.stdPostAPI("/tpool/raw", postValues)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(100, time.Millisecond*100, func() error {
		return st3.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
	})
	if err != nil {
		t.Fatal("Txn was not broadcast to the third server", err)
	}

	// Mine a block on the first server, which should clear its transaction
	// pool.
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = st.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
	if err.Error() != "transaction not found in transaction pool" {
		t.Fatal("transaction should be gone from the pool after mining a block")
	}

	// Convert the returned transactions to base64, which is how they will be
	// presetned to someone using curl. Submit those to the POST endpoint. The
	// POST endpoint should gracefully handle that submission as base64.
	//
	// The first 3 st's all have the transactions already, so now we'll use st4.
	b64Parents := base64.StdEncoding.EncodeToString(trg.Parents)
	b64Transaction := base64.StdEncoding.EncodeToString(trg.Transaction)
	postValues = url.Values{}
	postValues.Set("parents", b64Parents)
	postValues.Set("transaction", b64Transaction)
	err = st4.stdPostAPI("/tpool/raw", postValues)
	if err != nil {
		t.Fatal(err)
	}
}

// TestTransactionPoolFee tests the /tpool/fee endpoint.
func TestTransactionPoolFee(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	var fees TpoolFeeGET
	err = st.getAPI("/tpool/fee", &fees)
	if err != nil {
		t.Fatal(err)
	}

	min, max := st.tpool.FeeEstimation()
	if !min.Equals(fees.Minimum) || !max.Equals(fees.Maximum) {
		t.Fatal("fee mismatch")
	}
}

// TestTransactionPoolConfirmed tests the /tpool/confirmed endpoint.
func TestTransactionPoolConfirmed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a transaction.
	sentValue := types.SiacoinPrecision.Mul64(1000)
	txns, err := st.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	txnID := txns[len(txns)-1].ID().String()

	// Transaction should not be confirmed yet.
	var tcg TpoolConfirmedGET
	err = st.getAPI("/tpool/confirmed/"+txnID, &tcg)
	if err != nil {
		t.Fatal(err)
	} else if tcg.Confirmed {
		t.Fatal("transaction should not be confirmed")
	}

	// Mine the block containing the transaction
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Transaction should now be confirmed.
	err = st.getAPI("/tpool/confirmed/"+txnID, &tcg)
	if err != nil {
		t.Fatal(err)
	} else if !tcg.Confirmed {
		t.Fatal("transaction should be confirmed")
	}

	// Check for a nonexistent transaction.
	badID := strings.Repeat("0", len(txnID))
	err = st.getAPI("/tpool/confirmed/"+badID, &tcg)
	if err != nil {
		t.Fatal(err)
	} else if tcg.Confirmed {
		t.Fatal("transaction should not be confirmed")
	}
}
