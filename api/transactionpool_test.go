package api

import (
	"net/url"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// TestTransactionPoolRawHandlerGET verifies that the transaction pool's raw
// transaction getter endpoint works correctly.
func TestTransactionPoolRawHandlerGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// test getting a nonexistent transaction
	nonexistentID := types.Transaction{}.ID()
	var trg TpoolRawGET
	err = st.getAPI("/tpool/raw/"+nonexistentID.String(), &trg)
	if err == nil {
		t.Fatal("expected invalid transaction pool id to return an error")
	}
	if err.Error() != "transaction not found in transaction pool" {
		t.Fatal("/tpool/raw should return not found with nonexistent transaction ID")
	}

	// submit a wallet transaction
	sentValue := types.SiacoinPrecision.Mul64(1000)
	txns, err := st.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}

	// verify the txns are in the pool
	for _, txn := range txns {
		err = st.getAPI("/tpool/raw/"+txn.ID().String(), &trg)
		if err != nil {
			t.Fatal(err)
		}
	}

	// verify correct parents and txn are returned
	lastTxn := txns[len(txns)-1]
	err = st.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
	if err != nil {
		t.Fatal(err)
	}
	var decodedParents []types.Transaction
	err = encoding.Unmarshal(trg.Parents, &decodedParents)
	if err != nil {
		t.Fatal(err)
	}
	if len(decodedParents) != len(txns)-1 {
		t.Fatal("returned the incorrect number of parents")
	}
	var decodedTxn types.Transaction
	err = encoding.Unmarshal(trg.Transaction, &decodedTxn)
	if err != nil {
		t.Fatal(err)
	}
	if decodedTxn.ID() != lastTxn.ID() {
		t.Fatal("tpool raw get returned the wrong transaction")
	}

	// mine a block, removing the txn from the txn pool
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	err = st.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
	if err.Error() != "transaction not found in transaction pool" {
		t.Fatal("transaction should be gone from the pool after mining a block")
	}
}

// TestTransactionPoolRawHandlerPOST verifies that the transaction pools' raw
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
	defer st.server.panicClose()

	sentValue := types.SiacoinPrecision.Mul64(1000)
	txns, err := st.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}

	// spin up a new server tester that hasn't seen this txn
	st2, err := blankServerTester(t.Name() + "-st2")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.panicClose()

	err = fullyConnectNodes([]*serverTester{st, st2})
	if err != nil {
		t.Fatal(err)
	}

	lastTxn := txns[len(txns)-1]

	var trg TpoolRawGET
	err = st2.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
	if err.Error() != "transaction not found in transaction pool" {
		t.Fatal("transaction should be missing initially from the second tpool")
	}

	// should be able to get and rebroadcast this transaction
	err = st.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
	if err != nil {
		t.Fatal(err)
	}
	postValues := url.Values{}
	postValues.Set("parents", string(trg.Parents))
	postValues.Set("transaction", string(trg.Transaction))
	err = st.stdPostAPI("/tpool/raw", postValues)
	if err != nil {
		t.Fatal(err)
	}

	retry(60, time.Second, func() error {
		err = st2.getAPI("/tpool/raw/"+lastTxn.ID().String(), &trg)
		if err != nil {
			return err
		}
		return nil
	})
}

// TestTransactionPoolRawHandlersVerification verifies that the transaction
// pool's raw endpoints get and broadcast transactions correctly.
func TestTransactionPoolRawHandlersVerification(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create two servers, connect them, mine a few blocks
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()
	st2, err := blankServerTester(t.Name() + "-st2")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.panicClose()

	testGroup := []*serverTester{st, st2}

	err = fullyConnectNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, err = st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = synchronizationCheck(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// disconnect the servers, isolating them
	for _, tester := range testGroup {
		for _, peer := range tester.gateway.Peers() {
			tester.gateway.Disconnect(peer.NetAddress)
		}
	}

	// make a transaction on the disconnected server
	sentValue := types.SiacoinPrecision.Mul64(1000)
	txns, err := st.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}

	// use the tpool handlers to show the transaction to the other disconnected
	// server
	txn := txns[len(txns)-1]
	var trg TpoolRawGET
	err = st.getAPI("/tpool/raw/"+txn.ID().String(), &trg)
	if err != nil {
		t.Fatal(err)
	}
	postValues := url.Values{}
	postValues.Set("parents", string(trg.Parents))
	postValues.Set("transaction", string(trg.Transaction))
	err = st2.stdPostAPI("/tpool/raw", postValues)
	if err != nil {
		t.Fatal(err)
	}

	// make a new server and connect/sync it with st2
	st3, err := blankServerTester(t.Name() + "-st3")
	if err != nil {
		t.Fatal(err)
	}
	defer st3.server.panicClose()
	err = fullyConnectNodes([]*serverTester{st2, st3})
	if err != nil {
		t.Fatal(err)
	}

	// post the transaction to st2 again
	err = st2.stdPostAPI("/tpool/raw", postValues)
	if err != nil {
		t.Fatal(err)
	}

	// st3 should eventually see the transaction
	retry(60, time.Second, func() error {
		return st3.getAPI("/tpool/raw/"+txn.ID().String(), &trg)
	})
}
