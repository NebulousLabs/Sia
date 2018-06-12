package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/fastrand"
)

// TestWalletGETEncrypted probes the GET call to /wallet when the
// wallet has never been encrypted.
func TestWalletGETEncrypted(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Check a wallet that has never been encrypted.
	testdir := build.TempDir("api", t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal("Failed to create gateway:", err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal("Failed to create consensus set:", err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal("Failed to create tpool:", err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal("Failed to create wallet:", err)
	}
	srv, err := NewServer("localhost:0", "Sia-Agent", "", cs, nil, g, nil, nil, nil, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Assemble the serverTester and start listening for api requests.
	st := &serverTester{
		cs:      cs,
		gateway: g,
		tpool:   tp,
		wallet:  w,

		server: srv,
	}
	errChan := make(chan error)
	go func() {
		listenErr := srv.Serve()
		errChan <- listenErr
	}()
	defer func() {
		err := <-errChan
		if err != nil {
			t.Fatalf("API server quit: %v", err)
		}
	}()
	defer st.server.panicClose()

	var wg WalletGET
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}
	if wg.Encrypted {
		t.Error("Wallet has never been encrypted")
	}
	if wg.Unlocked {
		t.Error("Wallet has never been unlocked")
	}
}

// TestWalletEncrypt tries to encrypt and unlock the wallet through the api
// using a provided encryption key.
func TestWalletEncrypt(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	testdir := build.TempDir("api", t.Name())

	walletPassword := "testpass"
	key := crypto.TwofishKey(crypto.HashObject(walletPassword))

	st, err := assembleServerTester(key, testdir)
	if err != nil {
		t.Fatal(err)
	}

	// lock the wallet
	err = st.stdPostAPI("/wallet/lock", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Use the password to call /wallet/unlock.
	unlockValues := url.Values{}
	unlockValues.Set("encryptionpassword", walletPassword)
	err = st.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err := st.wallet.Unlocked()
	if err != nil {
		t.Error(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}

	// reload the server and verify unlocking still works
	err = st.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	st2, err := assembleServerTester(st.walletKey, st.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.panicClose()

	// lock the wallet
	err = st2.stdPostAPI("/wallet/lock", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Use the password to call /wallet/unlock.
	err = st2.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err = st2.wallet.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}
}

// TestWalletBlankEncrypt tries to encrypt and unlock the wallet
// through the api using a blank encryption key - meaning that the wallet seed
// returned by the encryption call can be used as the encryption key.
func TestWalletBlankEncrypt(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Create a server object without encrypting or unlocking the wallet.
	testdir := build.TempDir("api", t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer("localhost:0", "Sia-Agent", "", cs, nil, g, nil, nil, nil, tp, w)
	if err != nil {
		t.Fatal(err)
	}
	// Assemble the serverTester.
	st := &serverTester{
		cs:      cs,
		gateway: g,
		tpool:   tp,
		wallet:  w,
		server:  srv,
	}
	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			panic(listenErr)
		}
	}()
	defer st.server.panicClose()

	// Make a call to /wallet/init and get the seed. Provide no encryption
	// key so that the encryption key is the seed that gets returned.
	var wip WalletInitPOST
	err = st.postAPI("/wallet/init", url.Values{}, &wip)
	if err != nil {
		t.Fatal(err)
	}
	// Use the seed to call /wallet/unlock.
	unlockValues := url.Values{}
	unlockValues.Set("encryptionpassword", wip.PrimarySeed)
	err = st.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err := w.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}
}

// TestIntegrationWalletInitSeed tries to encrypt and unlock the wallet
// through the api using a supplied seed.
func TestIntegrationWalletInitSeed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a server object without encrypting or unlocking the wallet.
	testdir := build.TempDir("api", "TestIntegrationWalletInitSeed")
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer("localhost:0", "Sia-Agent", "", cs, nil, g, nil, nil, nil, tp, w)
	if err != nil {
		t.Fatal(err)
	}
	// Assemble the serverTester.
	st := &serverTester{
		cs:      cs,
		gateway: g,
		tpool:   tp,
		wallet:  w,
		server:  srv,
	}
	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			panic(listenErr)
		}
	}()
	defer st.server.panicClose()

	// Make a call to /wallet/init/seed using an invalid seed
	qs := url.Values{}
	qs.Set("seed", "foo")
	err = st.stdPostAPI("/wallet/init/seed", qs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Make a call to /wallet/init/seed. Provide no encryption key so that the
	// encryption key is the seed.
	var seed modules.Seed
	fastrand.Read(seed[:])
	seedStr, _ := modules.SeedToString(seed, "english")
	qs.Set("seed", seedStr)
	err = st.stdPostAPI("/wallet/init/seed", qs)
	if err != nil {
		t.Fatal(err)
	}

	// Try to re-init the wallet using a different encryption key
	qs.Set("encryptionpassword", "foo")
	err = st.stdPostAPI("/wallet/init/seed", qs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Use the seed to call /wallet/unlock.
	unlockValues := url.Values{}
	unlockValues.Set("encryptionpassword", seedStr)
	err = st.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err := w.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}
}

// TestWalletGETSiacoins probes the GET call to /wallet when the
// siacoin balance is being manipulated.
func TestWalletGETSiacoins(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Check the initial wallet is encrypted, unlocked, and has the siacoins
	// that got mined.
	var wg WalletGET
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}
	if !wg.Encrypted {
		t.Error("Wallet has been encrypted")
	}
	if !wg.Unlocked {
		t.Error("Wallet has been unlocked")
	}
	if wg.ConfirmedSiacoinBalance.Cmp(types.CalculateCoinbase(1)) != 0 {
		t.Error("reported wallet balance does not reflect the single block that has been mined")
	}
	if wg.UnconfirmedOutgoingSiacoins.Cmp64(0) != 0 {
		t.Error("there should not be unconfirmed outgoing siacoins")
	}
	if wg.UnconfirmedIncomingSiacoins.Cmp64(0) != 0 {
		t.Error("there should not be unconfirmed incoming siacoins")
	}

	// Send coins to a wallet address through the api.
	var wag WalletAddressGET
	err = st.getAPI("/wallet/address", &wag)
	if err != nil {
		t.Fatal(err)
	}
	sendSiacoinsValues := url.Values{}
	sendSiacoinsValues.Set("amount", "1234")
	sendSiacoinsValues.Set("destination", wag.Address.String())
	err = st.stdPostAPI("/wallet/siacoins", sendSiacoinsValues)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet is reporting unconfirmed siacoins.
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}
	if !wg.Encrypted {
		t.Error("Wallet has been encrypted")
	}
	if !wg.Unlocked {
		t.Error("Wallet has been unlocked")
	}
	if wg.ConfirmedSiacoinBalance.Cmp(types.CalculateCoinbase(1)) != 0 {
		t.Error("reported wallet balance does not reflect the single block that has been mined")
	}
	if wg.UnconfirmedOutgoingSiacoins.Cmp64(0) <= 0 {
		t.Error("there should be unconfirmed outgoing siacoins")
	}
	if wg.UnconfirmedIncomingSiacoins.Cmp64(0) <= 0 {
		t.Error("there should be unconfirmed incoming siacoins")
	}
	if wg.UnconfirmedOutgoingSiacoins.Cmp(wg.UnconfirmedIncomingSiacoins) <= 0 {
		t.Error("net movement of siacoins should be outgoing (miner fees)")
	}

	// Mine a block and see that the unconfirmed balances reduce back to
	// nothing.
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}
	if wg.ConfirmedSiacoinBalance.Cmp(types.CalculateCoinbase(1).Add(types.CalculateCoinbase(2))) >= 0 {
		t.Error("reported wallet balance does not reflect mining two blocks and eating a miner fee")
	}
	if wg.UnconfirmedOutgoingSiacoins.Cmp64(0) != 0 {
		t.Error("there should not be unconfirmed outgoing siacoins")
	}
	if wg.UnconfirmedIncomingSiacoins.Cmp64(0) != 0 {
		t.Error("there should not be unconfirmed incoming siacoins")
	}
}

// TestIntegrationWalletSweepSeedPOST probes the POST call to
// /wallet/sweep/seed.
func TestIntegrationWalletSweepSeedPOST(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// send coins to a new wallet, then sweep them back
	key := crypto.GenerateTwofishKey()
	w, err := wallet.New(st.cs, st.tpool, filepath.Join(st.dir, "wallet2"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Encrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	err = w.Unlock(key)
	if err != nil {
		t.Fatal(err)
	}
	addr, _ := w.NextAddress()
	st.wallet.SendSiacoins(types.SiacoinPrecision.Mul64(100), addr.UnlockHash())
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	seed, _, _ := w.PrimarySeed()
	seedStr, _ := modules.SeedToString(seed, "english")

	// Sweep the coins we sent
	var wsp WalletSweepPOST
	qs := url.Values{}
	qs.Set("seed", seedStr)
	err = st.postAPI("/wallet/sweep/seed", qs, &wsp)
	if err != nil {
		t.Fatal(err)
	}
	// Should have swept more than 80 SC
	if wsp.Coins.Cmp(types.SiacoinPrecision.Mul64(80)) <= 0 {
		t.Fatalf("swept fewer coins (%v SC) than expected %v+", wsp.Coins.Div(types.SiacoinPrecision), 80)
	}

	// Add a block so that the sweep transaction is processed
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Sweep again; should find no coins. An error will be returned because
	// the found coins cannot cover the transaction fee.
	err = st.postAPI("/wallet/sweep/seed", qs, &wsp)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Call /wallet/sweep/seed with an invalid seed
	qs.Set("seed", "foo")
	err = st.postAPI("/wallet/sweep/seed", qs, &wsp)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestIntegrationWalletLoadSeedPOST probes the POST call to
// /wallet/seed.
func TestIntegrationWalletLoadSeedPOST(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a wallet.
	key := crypto.TwofishKey(crypto.HashObject("password"))
	st, err := assembleServerTester(key, build.TempDir("api", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer st.panicClose()
	// Mine blocks until the wallet has confirmed money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create a wallet to load coins from.
	key2 := crypto.GenerateTwofishKey()
	w2, err := wallet.New(st.cs, st.tpool, filepath.Join(st.dir, "wallet2"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = w2.Encrypt(key2)
	if err != nil {
		t.Fatal(err)
	}
	err = w2.Unlock(key2)
	if err != nil {
		t.Fatal(err)
	}
	// Mine coins into the second wallet.
	m, err := miner.New(st.cs, st.tpool, w2, filepath.Join(st.dir, "miner2"))
	if err != nil {
		t.Fatal(err)
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Record starting balances.
	oldBal, _, _, err1 := st.wallet.ConfirmedBalance()
	w2bal, _, _, err2 := w2.ConfirmedBalance()
	if errs := errors.Compose(err1, err2); errs != nil {
		t.Fatal(errs)
	}
	if w2bal.IsZero() {
		t.Fatal("second wallet's balance should not be zero")
	}

	// Load the second wallet's seed into the first wallet
	seed, _, _ := w2.PrimarySeed()
	seedStr, _ := modules.SeedToString(seed, "english")
	qs := url.Values{}
	qs.Set("seed", seedStr)
	qs.Set("encryptionpassword", "password")
	err = st.stdPostAPI("/wallet/seed", qs)
	if err != nil {
		t.Fatal(err)
	}
	// First wallet should now have balance of both wallets
	bal, _, _, err := st.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	if exp := oldBal.Add(w2bal); !bal.Equals(exp) {
		t.Fatalf("wallet did not load seed correctly: expected %v coins, got %v", exp, bal)
	}
}

// TestWalletTransactionGETid queries the /wallet/transaction/:id
// api call.
func TestWalletTransactionGETid(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Mining blocks should have created transactions for the wallet containing
	// miner payouts. Get the list of transactions.
	var wtg WalletTransactionsGET
	err = st.getAPI("/wallet/transactions?startheight=0&endheight=10", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtg.ConfirmedTransactions) == 0 {
		t.Error("expecting a few wallet transactions, corresponding to miner payouts.")
	}
	if len(wtg.UnconfirmedTransactions) != 0 {
		t.Error("expecting 0 unconfirmed transactions")
	}
	// A call to /wallet/transactions without startheight and endheight parameters
	// should return a descriptive error message.
	err = st.getAPI("/wallet/transactions", &wtg)
	if err == nil || err.Error() != "startheight and endheight must be provided to a /wallet/transactions call." {
		t.Error("expecting /wallet/transactions call with empty parameters to error")
	}

	// Query the details of the first transaction using
	// /wallet/transaction/:id
	var wtgid WalletTransactionGETid
	wtgidQuery := fmt.Sprintf("/wallet/transaction/%s", wtg.ConfirmedTransactions[0].TransactionID)
	err = st.getAPI(wtgidQuery, &wtgid)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtgid.Transaction.Inputs) != 0 {
		t.Error("miner payout should appear as an output, not an input")
	}
	if len(wtgid.Transaction.Outputs) != 1 {
		t.Fatal("a single miner payout output should have been created")
	}
	if wtgid.Transaction.Outputs[0].FundType != types.SpecifierMinerPayout {
		t.Error("fund type should be a miner payout")
	}
	if wtgid.Transaction.Outputs[0].Value.IsZero() {
		t.Error("output should have a nonzero value")
	}

	// Query the details of a transaction where siacoins were sent.
	//
	// NOTE: We call the SendSiacoins method directly to get convenient access
	// to the txid.
	sentValue := types.SiacoinPrecision.Mul64(3)
	txns, err := st.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	var wtgid2 WalletTransactionGETid
	err = st.getAPI(fmt.Sprintf("/wallet/transaction/%s", txns[1].ID()), &wtgid2)
	if err != nil {
		t.Fatal(err)
	}
	txn := wtgid2.Transaction
	if txn.TransactionID != txns[1].ID() {
		t.Error("wrong transaction was fetched")
	} else if len(txn.Inputs) != 1 || len(txn.Outputs) != 2 {
		t.Error("expected 1 input and 2 outputs, got", len(txn.Inputs), len(txn.Outputs))
	} else if !txn.Outputs[0].Value.Equals(sentValue) {
		t.Errorf("expected first output to equal %v, got %v", sentValue, txn.Outputs[0].Value)
	} else if exp := txn.Inputs[0].Value.Sub(sentValue); !txn.Outputs[1].Value.Equals(exp) {
		t.Errorf("expected first output to equal %v, got %v", exp, txn.Outputs[1].Value)
	}

	// Create a second wallet and send money to that wallet.
	st2, err := blankServerTester(t.Name() + "w2")
	if err != nil {
		t.Fatal(err)
	}
	err = fullyConnectNodes([]*serverTester{st, st2})
	if err != nil {
		t.Fatal(err)
	}

	// Send a transaction from the one wallet to the other.
	var wag WalletAddressGET
	err = st2.getAPI("/wallet/address", &wag)
	if err != nil {
		t.Fatal(err)
	}
	sendSiacoinsValues := url.Values{}
	sendSiacoinsValues.Set("amount", sentValue.String())
	sendSiacoinsValues.Set("destination", wag.Address.String())
	err = st.stdPostAPI("/wallet/siacoins", sendSiacoinsValues)
	if err != nil {
		t.Fatal(err)
	}

	// Check the unconfirmed transactions in the sending wallet to see the id of
	// the output being spent.
	err = st.getAPI("/wallet/transactions?startheight=0&endheight=10000", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtg.UnconfirmedTransactions) != 2 {
		t.Fatal("expecting two unconfirmed transactions in sender wallet")
	}
	// Check that undocumented API behaviour used in Sia-UI still works with
	// current API.
	err = st.getAPI("/wallet/transactions?startheight=0&endheight=-1", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtg.UnconfirmedTransactions) != 2 {
		t.Fatal("expecting two unconfirmed transactions in sender wallet")
	}
	// Get the id of the non-change output sent to the receiving wallet.
	expectedOutputID := wtg.UnconfirmedTransactions[1].Outputs[0].ID

	// Check the unconfirmed transactions struct to make sure all fields are
	// filled out correctly in the receiving wallet.
	err = st2.getAPI("/wallet/transactions?startheight=0&endheight=10000", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	// There should be at least one unconfirmed transaction:
	err = build.Retry(50, time.Millisecond*100, func() error {
		if len(wtg.UnconfirmedTransactions) < 1 {
			return errors.New("unconfirmed transaction not found")
		}
		return nil
	})
	// The unconfirmed transaction should have inputs and outputs, and both of
	// those should have value.
	for _, txn := range wtg.UnconfirmedTransactions {
		if len(txn.Inputs) < 1 {
			t.Fatal("transaction should have an input")
		}
		if len(txn.Outputs) < 1 {
			t.Fatal("transactions should have outputs")
		}
		for _, input := range txn.Inputs {
			if input.Value.IsZero() {
				t.Error("input should not have zero value")
			}
		}
		for _, output := range txn.Outputs {
			if output.Value.IsZero() {
				t.Error("output should not have zero value")
			}
		}
		if txn.Outputs[0].ID != expectedOutputID {
			t.Error("transactions should have matching output ids for the same transaction")
		}
	}

	// Restart st2.
	err = st2.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	st2, err = assembleServerTester(st2.walletKey, st2.dir)
	if err != nil {
		t.Fatal(err)
	}
	err = st2.getAPI("/wallet/transactions?startheight=0&endheight=10000", &wtg)
	if err != nil {
		t.Fatal(err)
	}

	// Reconnect st2 and st.
	err = fullyConnectNodes([]*serverTester{st, st2})
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block on st to get the transactions into the blockchain.
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = synchronizationCheck([]*serverTester{st, st2})
	if err != nil {
		t.Fatal(err)
	}
	err = st2.getAPI("/wallet/transactions?startheight=0&endheight=10000", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	// There should be at least one confirmed transaction:
	if len(wtg.ConfirmedTransactions) < 1 {
		t.Fatal("confirmed transaction not found")
	}
	for _, txn := range wtg.ConfirmedTransactions {
		if len(txn.Inputs) < 1 {
			t.Fatal("transaction should have an input")
		}
		if len(txn.Outputs) < 1 {
			t.Fatal("transactions should have outputs")
		}
		for _, input := range txn.Inputs {
			if input.Value.IsZero() {
				t.Error("input should not have zero value")
			}
		}
		for _, output := range txn.Outputs {
			if output.Value.IsZero() {
				t.Error("output should not have zero value")
			}
		}
	}

	// Reset the wallet and see that the confirmed transactions are still there.
	err = st2.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	st2, err = assembleServerTester(st2.walletKey, st2.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.Close()
	err = st2.getAPI("/wallet/transactions?startheight=0&endheight=10000", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	// There should be at least one confirmed transaction:
	if len(wtg.ConfirmedTransactions) < 1 {
		t.Fatal("unconfirmed transaction not found")
	}
	// Check whether the confirmed transactions remain.
	for _, txn := range wtg.ConfirmedTransactions {
		if len(txn.Inputs) < 1 {
			t.Fatal("transaction should have an input")
		}
		if len(txn.Outputs) < 1 {
			t.Fatal("transactions should have outputs")
		}
		for _, input := range txn.Inputs {
			if input.Value.IsZero() {
				t.Error("input should not have zero value")
			}
		}
		for _, output := range txn.Outputs {
			if output.Value.IsZero() {
				t.Error("output should not have zero value")
			}
		}
	}
}

// Tests that the /wallet/backup call checks for relative paths.
func TestWalletRelativePathErrorBackup(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}

	// Create tmp directory for uploads/downloads.
	walletTestDir := build.TempDir("wallet_relative_path_backup")
	err = os.MkdirAll(walletTestDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Wallet backup should error if its destination is a relative path
	backupAbsoluteError := "error when calling /wallet/backup: destination must be an absolute path"
	// This should error.
	err = st.stdGetAPI("/wallet/backup?destination=test_wallet.backup")
	if err == nil || err.Error() != backupAbsoluteError {
		t.Fatal(err)
	}
	// This as well.
	err = st.stdGetAPI("/wallet/backup?destination=../test_wallet.backup")
	if err == nil || err.Error() != backupAbsoluteError {
		t.Fatal(err)
	}
	// This should succeed.
	err = st.stdGetAPI("/wallet/backup?destination=" + filepath.Join(walletTestDir, "test_wallet.backup"))
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the backup was actually created.
	_, errStat := os.Stat(filepath.Join(walletTestDir, "test_wallet.backup"))
	if errStat != nil {
		t.Error(errStat)
	}
}

// Tests that the /wallet/033x call checks for relative paths.
func TestWalletRelativePathError033x(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}

	// Create tmp directory for uploads/downloads.
	walletTestDir := build.TempDir("wallet_relative_path_033x")
	err = os.MkdirAll(walletTestDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Wallet loading from 033x should error if its source is a relative path
	load033xAbsoluteError := "error when calling /wallet/033x: source must be an absolute path"

	// This should fail.
	load033xValues := url.Values{}
	load033xValues.Set("source", "test.dat")
	err = st.stdPostAPI("/wallet/033x", load033xValues)
	if err == nil || err.Error() != load033xAbsoluteError {
		t.Fatal(err)
	}

	// As should this.
	load033xValues = url.Values{}
	load033xValues.Set("source", "../test.dat")
	err = st.stdPostAPI("/wallet/033x", load033xValues)
	if err == nil || err.Error() != load033xAbsoluteError {
		t.Fatal(err)
	}

	// This should succeed (though the wallet method will still return an error)
	load033xValues = url.Values{}
	if err = createRandFile(filepath.Join(walletTestDir, "test.dat"), 0); err != nil {
		t.Fatal(err)
	}
	load033xValues.Set("source", filepath.Join(walletTestDir, "test.dat"))
	err = st.stdPostAPI("/wallet/033x", load033xValues)
	if err == nil || err.Error() == load033xAbsoluteError {
		t.Fatal(err)
	}
}

// Tests that the /wallet/siagkey call checks for relative paths.
func TestWalletRelativePathErrorSiag(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}

	// Create tmp directory for uploads/downloads.
	walletTestDir := build.TempDir("wallet_relative_path_sig")
	err = os.MkdirAll(walletTestDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Wallet loading from siag should error if its source is a relative path
	loadSiagAbsoluteError := "error when calling /wallet/siagkey: keyfiles contains a non-absolute path"

	// This should fail.
	loadSiagValues := url.Values{}
	loadSiagValues.Set("keyfiles", "test.dat")
	err = st.stdPostAPI("/wallet/siagkey", loadSiagValues)
	if err == nil || err.Error() != loadSiagAbsoluteError {
		t.Fatal(err)
	}

	// As should this.
	loadSiagValues = url.Values{}
	loadSiagValues.Set("keyfiles", "../test.dat")
	err = st.stdPostAPI("/wallet/siagkey", loadSiagValues)
	if err == nil || err.Error() != loadSiagAbsoluteError {
		t.Fatal(err)
	}

	// This should fail.
	loadSiagValues = url.Values{}
	loadSiagValues.Set("keyfiles", "/test.dat,test.dat,../test.dat")
	err = st.stdPostAPI("/wallet/siagkey", loadSiagValues)
	if err == nil || err.Error() != loadSiagAbsoluteError {
		t.Fatal(err)
	}

	// As should this.
	loadSiagValues = url.Values{}
	loadSiagValues.Set("keyfiles", "../test.dat,/test.dat")
	err = st.stdPostAPI("/wallet/siagkey", loadSiagValues)
	if err == nil || err.Error() != loadSiagAbsoluteError {
		t.Fatal(err)
	}

	// This should succeed.
	loadSiagValues = url.Values{}
	if err = createRandFile(filepath.Join(walletTestDir, "test.dat"), 0); err != nil {
		t.Fatal(err)
	}
	loadSiagValues.Set("keyfiles", filepath.Join(walletTestDir, "test.dat"))
	err = st.stdPostAPI("/wallet/siagkey", loadSiagValues)
	if err == nil || err.Error() == loadSiagAbsoluteError {
		t.Fatal(err)
	}

	// As should this.
	loadSiagValues = url.Values{}
	if err = createRandFile(filepath.Join(walletTestDir, "test1.dat"), 0); err != nil {
		t.Fatal(err)
	}
	loadSiagValues.Set("keyfiles", filepath.Join(walletTestDir, "test.dat")+","+filepath.Join(walletTestDir, "test1.dat"))
	err = st.stdPostAPI("/wallet/siagkey", loadSiagValues)
	if err == nil || err.Error() == loadSiagAbsoluteError {
		t.Fatal(err)
	}
}

func TestWalletReset(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	testdir := build.TempDir("api", t.Name())

	walletPassword := "testpass"
	key := crypto.TwofishKey(crypto.HashObject(walletPassword))

	st, err := assembleServerTester(key, testdir)
	if err != nil {
		t.Fatal(err)
	}

	// lock the wallet
	err = st.stdPostAPI("/wallet/lock", nil)
	if err != nil {
		t.Fatal(err)
	}

	// reencrypt the wallet
	newPassword := "testpass2"
	newKey := crypto.TwofishKey(crypto.HashObject(newPassword))

	initValues := url.Values{}
	initValues.Set("force", "true")
	initValues.Set("encryptionpassword", newPassword)
	err = st.stdPostAPI("/wallet/init", initValues)
	if err != nil {
		t.Fatal(err)
	}

	// Use the password to call /wallet/unlock.
	unlockValues := url.Values{}
	unlockValues.Set("encryptionpassword", newPassword)
	err = st.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err := st.wallet.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}

	// reload the server and verify unlocking still works
	err = st.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	st2, err := assembleServerTester(newKey, st.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.panicClose()

	// lock the wallet
	err = st2.stdPostAPI("/wallet/lock", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Use the password to call /wallet/unlock.
	err = st2.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err = st2.wallet.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}
}

func TestWalletSiafunds(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	walletPassword := "testpass"
	key := crypto.TwofishKey(crypto.HashObject(walletPassword))
	testdir := build.TempDir("api", t.Name())
	st, err := assembleServerTester(key, testdir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// mine some money
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// record transactions
	var wtg WalletTransactionsGET
	err = st.getAPI("/wallet/transactions?startheight=0&endheight=100", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	numTxns := len(wtg.ConfirmedTransactions)

	// load siafunds into the wallet
	siagPath, _ := filepath.Abs("../../types/siag0of1of1.siakey")
	loadSiagValues := url.Values{}
	loadSiagValues.Set("keyfiles", siagPath)
	loadSiagValues.Set("encryptionpassword", walletPassword)
	err = st.stdPostAPI("/wallet/siagkey", loadSiagValues)
	if err != nil {
		t.Fatal(err)
	}

	err = st.getAPI("/wallet/transactions?startheight=0&endheight=100", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtg.ConfirmedTransactions) != numTxns+1 {
		t.Errorf("expected %v transactions, got %v", numTxns+1, len(wtg.ConfirmedTransactions))
	}

	// check balance
	var wg WalletGET
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}
	if wg.SiafundBalance.Cmp64(2000) != 0 {
		t.Fatalf("bad siafund balance: expected %v, got %v", 2000, wg.SiafundBalance)
	}

	// spend the siafunds into the wallet seed
	var wag WalletAddressGET
	err = st.getAPI("/wallet/address", &wag)
	if err != nil {
		t.Fatal(err)
	}
	sendSiafundsValues := url.Values{}
	sendSiafundsValues.Set("amount", "2000")
	sendSiafundsValues.Set("destination", wag.Address.String())
	err = st.stdPostAPI("/wallet/siafunds", sendSiafundsValues)
	if err != nil {
		t.Fatal(err)
	}

	// Announce the host and form an allowance with it. This will result in a
	// siafund claim.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	err = st.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = st.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	// mine a block so that the announcement makes it into the blockchain
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// form allowance
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "20"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Block until allowance has finished forming.
	err = build.Retry(50, time.Millisecond*250, func() error {
		var rc RenterContracts
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 1 {
			return errors.New("no contracts")
		}
		return nil
	})
	if err != nil {
		t.Fatal("allowance setting failed")
	}

	// mine a block so that the file contract makes it into the blockchain
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// wallet should now have a claim balance
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}
	if wg.SiacoinClaimBalance.IsZero() {
		t.Fatal("expected non-zero claim balance")
	}
}

// TestWalletVerifyAddress tests that the /wallet/verify/address/:addr endpoint
// validates wallet addresses correctly.
func TestWalletVerifyAddress(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	var res WalletVerifyAddressGET
	fakeaddr := "thisisaninvalidwalletaddress"
	if err = st.getAPI("/wallet/verify/address/"+fakeaddr, &res); err != nil {
		t.Fatal(err)
	}
	if res.Valid == true {
		t.Fatal("expected /wallet/verify to fail an invalid address")
	}

	var wag WalletAddressGET
	err = st.getAPI("/wallet/address", &wag)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/wallet/verify/address/"+wag.Address.String(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Valid == false {
		t.Fatal("expected /wallet/verify to pass a valid address")
	}
}

// TestWalletChangePassword verifies that the /wallet/changepassword endpoint
// works correctly and changes a wallet password.
func TestWalletChangePassword(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	testdir := build.TempDir("api", t.Name())

	originalPassword := "testpass"
	newPassword := "newpass"
	originalKey := crypto.TwofishKey(crypto.HashObject(originalPassword))
	newKey := crypto.TwofishKey(crypto.HashObject(newPassword))

	st, err := assembleServerTester(originalKey, testdir)
	if err != nil {
		t.Fatal(err)
	}

	// lock the wallet
	err = st.stdPostAPI("/wallet/lock", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Use the password to call /wallet/unlock.
	unlockValues := url.Values{}
	unlockValues.Set("encryptionpassword", originalPassword)
	err = st.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err := st.wallet.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}

	// change the wallet key
	changeKeyValues := url.Values{}
	changeKeyValues.Set("encryptionpassword", originalPassword)
	changeKeyValues.Set("newpassword", newPassword)
	err = st.stdPostAPI("/wallet/changepassword", changeKeyValues)
	if err != nil {
		t.Fatal(err)
	}
	// wallet should still be unlocked
	unlocked, err = st.wallet.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Fatal("changepassword locked the wallet")
	}

	// lock the wallet and verify unlocking works with the new password
	err = st.stdPostAPI("/wallet/lock", nil)
	if err != nil {
		t.Fatal(err)
	}
	unlockValues.Set("encryptionpassword", newPassword)
	err = st.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err = st.wallet.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}

	// reload the server and verify unlocking still works
	err = st.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	st2, err := assembleServerTester(newKey, st.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.panicClose()

	// lock the wallet
	err = st2.stdPostAPI("/wallet/lock", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Use the password to call /wallet/unlock.
	err = st2.stdPostAPI("/wallet/unlock", unlockValues)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet actually unlocked.
	unlocked, err = st2.wallet.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked {
		t.Error("wallet is not unlocked")
	}
}

// TestWalletSiacoins tests the /wallet/siacoins endpoint, including sending
// to multiple addresses.
func TestWalletSiacoins(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()
	st2, err := blankServerTester(t.Name() + "-wallet2")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.Close()
	st3, err := blankServerTester(t.Name() + "-wallet3")
	if err != nil {
		t.Fatal(err)
	}
	defer st3.server.Close()
	st4, err := blankServerTester(t.Name() + "-wallet4")
	if err != nil {
		t.Fatal(err)
	}
	defer st4.server.Close()
	st5, err := blankServerTester(t.Name() + "-wallet5")
	if err != nil {
		t.Fatal(err)
	}
	defer st5.server.Close()
	st6, err := blankServerTester(t.Name() + "-wallet6")
	if err != nil {
		t.Fatal(err)
	}
	defer st6.server.Close()

	// Mine two more blocks with 'st' to get extra outputs to spend.
	for i := 0; i < 2; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Connect all the wallets together.
	wallets := []*serverTester{st, st2, st3, st4, st5, st6}
	err = fullyConnectNodes(wallets)
	if err != nil {
		t.Fatal(err)
	}

	// Send 10KS in a single-send to st2.
	sendAmount := types.SiacoinPrecision.Mul64(10000)
	var wag WalletAddressGET
	err = st2.getAPI("/wallet/address", &wag)
	if err != nil {
		t.Fatal(err)
	}
	sendSiacoinsValues := url.Values{}
	outputsJSON, _ := json.Marshal([]types.SiacoinOutput{{
		UnlockHash: wag.Address,
		Value:      sendAmount,
	}})
	sendSiacoinsValues.Set("outputs", string(outputsJSON))
	if err = st.stdPostAPI("/wallet/siacoins", sendSiacoinsValues); err != nil {
		t.Fatal(err)
	}

	// Send 10KS to 3, 4, 5 in a single send.
	var outputs []types.SiacoinOutput
	for _, w := range wallets[2:5] {
		var wag WalletAddressGET
		err = w.getAPI("/wallet/address", &wag)
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, types.SiacoinOutput{
			UnlockHash: wag.Address,
			Value:      sendAmount,
		})
	}
	outputsJSON, _ = json.Marshal(outputs)
	sendSiacoinsValues = url.Values{}
	sendSiacoinsValues.Set("outputs", string(outputsJSON))
	if err = st.stdPostAPI("/wallet/siacoins", sendSiacoinsValues); err != nil {
		t.Fatal(err)
	}

	// Send 10KS to 6 through a joined 250 sends.
	outputs = nil
	smallSend := sendAmount.Div64(250)
	for i := 0; i < 250; i++ {
		var wag WalletAddressGET
		err = st6.getAPI("/wallet/address", &wag)
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, types.SiacoinOutput{
			UnlockHash: wag.Address,
			Value:      smallSend,
		})
	}
	outputsJSON, _ = json.Marshal(outputs)
	sendSiacoinsValues = url.Values{}
	sendSiacoinsValues.Set("outputs", string(outputsJSON))
	if err = st.stdPostAPI("/wallet/siacoins", sendSiacoinsValues); err != nil {
		t.Fatal(err)
	}

	// Mine a block to confirm the send.
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Wait for the block to propagate.
	_, err = synchronizationCheck(wallets)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallets all have 10KS.
	for i, w := range wallets[1:] {
		var wg WalletGET
		err = w.getAPI("/wallet", &wg)
		if err != nil {
			t.Fatal(err)
		}
		if !wg.ConfirmedSiacoinBalance.Equals(sendAmount) {
			t.Errorf("wallet %d should have %v coins, has %v", i+2, sendAmount, wg.ConfirmedSiacoinBalance)
		}
	}
}

// TestWalletGETDust tests the consistency of dustthreshold field in /wallet
func TestWalletGETDust(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	var wg WalletGET
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}

	dt, err := st.wallet.DustThreshold()
	if err != nil {
		t.Fatal(err)
	}
	if !dt.Equals(wg.DustThreshold) {
		t.Fatal("dustThreshold mismatch")
	}
}

// testWalletTransactionEndpoint is a subtest that queries the transaction endpoint of a node.
func testWalletTransactionEndpoint(t *testing.T, st *serverTester, expectedConfirmedTxns int) {
	// Mining blocks should have created transactions for the wallet containing
	// miner payouts. Get the list of transactions.
	var wtg WalletTransactionsGET
	err := st.getAPI("/wallet/transactions?startheight=0&endheight=-1", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtg.ConfirmedTransactions) != expectedConfirmedTxns {
		t.Fatalf("expected %v txns but was %v", expectedConfirmedTxns, len(wtg.ConfirmedTransactions))
	}

	// Query the details of all transactions using
	// /wallet/transaction/:id
	for _, txn := range wtg.ConfirmedTransactions {
		var wtgid WalletTransactionGETid
		wtgidQuery := fmt.Sprintf("/wallet/transaction/%s", txn.TransactionID)
		err = st.getAPI(wtgidQuery, &wtgid)
		if err != nil {
			t.Fatal(err)
		}
		if wtgid.Transaction.TransactionID != txn.TransactionID {
			t.Fatalf("Expected txn with id %v but was %v", txn.TransactionID, wtgid.Transaction.TransactionID)
		}
	}
}

// testWalletTransactionEndpoint is a subtest that queries the transactions endpoint of a node.
func testWalletTransactionsEndpoint(t *testing.T, st *serverTester, expectedConfirmedTxns int) {
	var wtg WalletTransactionsGET
	err := st.getAPI("/wallet/transactions?startheight=0&endheight=-1", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtg.ConfirmedTransactions) != expectedConfirmedTxns {
		t.Fatalf("expected %v txns but was %v", expectedConfirmedTxns, len(wtg.ConfirmedTransactions))
	}
	totalTxns := len(wtg.ConfirmedTransactions)

	// Query the details of all transactions one block at a time using
	// /wallet/transactions
	queriedTxns := 0
	for i := types.BlockHeight(0); i <= st.cs.Height(); i++ {
		err := st.getAPI(fmt.Sprintf("/wallet/transactions?startheight=%v&endheight=%v", i, i), &wtg)
		if err != nil {
			t.Fatal(err)
		}
		queriedTxns += len(wtg.ConfirmedTransactions)
	}
	if queriedTxns != totalTxns {
		t.Errorf("Expected %v txns but was %v", totalTxns, queriedTxns)
	}

	queriedTxns = 0
	batchSize := types.BlockHeight(5)
	for i := types.BlockHeight(0); i <= st.cs.Height(); i += (batchSize + 1) {
		err := st.getAPI(fmt.Sprintf("/wallet/transactions?startheight=%v&endheight=%v", i, i+batchSize), &wtg)
		if err != nil {
			t.Fatal(err)
		}
		queriedTxns += len(wtg.ConfirmedTransactions)
	}
	if queriedTxns != totalTxns {
		t.Errorf("Expected %v txns but was %v", totalTxns, queriedTxns)
	}
}

// TestWalletManyTransactions creates a wallet and sends a large number of
// coins to itself. Afterwards it will execute subtests to test the wallet's
// scalability.
func TestWalletManyTransactions(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}

	// Declare tests that should be executed
	subtests := []struct {
		name string
		f    func(*testing.T, *serverTester, int)
	}{
		{"TestWalletTransactionEndpoint", testWalletTransactionEndpoint},
		{"TestWalletTransactionsEndpoint", testWalletTransactionsEndpoint},
	}

	// Create tester
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Disable defrag for the wallet
	st.wallet.SetSettings(modules.WalletSettings{
		NoDefrag: true,
	})

	// Mining blocks should have created transactions for the wallet containing
	// miner payouts. Get the list of transactions.
	var wtg WalletTransactionsGET
	err = st.getAPI("/wallet/transactions?startheight=0&endheight=-1", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtg.ConfirmedTransactions) == 0 {
		t.Fatal("expecting a few wallet transactions, corresponding to miner payouts.")
	}
	if len(wtg.UnconfirmedTransactions) != 0 {
		t.Fatal("expecting 0 unconfirmed transactions")
	}

	// Remember the number of confirmed transactions
	numConfirmedTxns := len(wtg.ConfirmedTransactions)

	// Get lots of addresses from the wallet
	numTxns := uint64(10000)
	ucs, err := st.wallet.NextAddresses(numTxns)
	if err != nil {
		t.Fatal(err)
	}

	// Send SC to each address.
	minedBlocks := 0
	for i, uc := range ucs {
		st.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
		if i%100 == 0 {
			if _, err := st.miner.AddBlock(); err != nil {
				t.Fatal(err)
			}
			minedBlocks++
		}
	}
	if _, err := st.miner.AddBlock(); err != nil {
		t.Fatal(err)
	}
	minedBlocks++

	// After sending numTxns times there should be 2*numTxns confirmed
	// transactions plus one for each mined block. Every send creates a setup
	// transaction and the actual transaction.
	err = st.getAPI("/wallet/transactions?startheight=0&endheight=-1", &wtg)
	if err != nil {
		t.Fatal(err)
	}
	expectedConfirmedTxns := numConfirmedTxns + int(2*numTxns) + minedBlocks
	if len(wtg.ConfirmedTransactions) != expectedConfirmedTxns {
		t.Fatalf("expecting %v confirmed transactions but was %v", expectedConfirmedTxns,
			len(wtg.ConfirmedTransactions))
	}
	if len(wtg.UnconfirmedTransactions) != 0 {
		t.Fatal("expecting 0 unconfirmed transactions")
	}

	// Execute tests
	for _, subtest := range subtests {
		t.Run(subtest.name, func(t *testing.T) {
			subtest.f(t, st, expectedConfirmedTxns)
		})
	}
}

// TestWalletTransactionsGETAddr queries the /wallet/transactions/:addr api
// call.
func TestWalletTransactionsGetAddr(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Create a second wallet.
	st2, err := blankServerTester(t.Name() + "w2")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.server.panicClose()

	err = fullyConnectNodes([]*serverTester{st, st2})
	if err != nil {
		t.Fatal(err)
	}

	// Get address of recipient
	uc, err := st2.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	addr := uc.UnlockHash()

	// Sent some money to the address
	sentValue := types.SiacoinPrecision.Mul64(3)
	_, err = st.wallet.SendSiacoins(sentValue, addr)
	if err != nil {
		t.Fatal(err)
	}

	// Query the details of the first transaction using
	// /wallet/transactions/:addr
	var wtga WalletTransactionsGETaddr
	wtgaQuery := fmt.Sprintf("/wallet/transactions/%s", addr)
	err = st.getAPI(wtgaQuery, &wtga)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtga.UnconfirmedTransactions) != 1 || len(wtga.ConfirmedTransactions) != 0 {
		t.Errorf("There should be exactly 1 unconfirmed and 0 confirmed related txns")
	}

	// Mine a block to get the transaction confirmed
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// See if they moved to the confirmed transactions after mining a block
	err = st.getAPI(wtgaQuery, &wtga)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtga.UnconfirmedTransactions) != 0 || len(wtga.ConfirmedTransactions) != 1 {
		t.Errorf("There should be exactly 0 unconfirmed and 1 confirmed related txns")
	}
}
