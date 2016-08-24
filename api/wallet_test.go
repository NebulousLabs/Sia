package api

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationWalletGETEncrypted probes the GET call to /wallet when the
// wallet has never been encrypted.
func TestIntegrationWalletGETEncrypted(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Check a wallet that has never been encrypted.
	testdir := build.TempDir("api", "TestIntegrationWalletGETEncrypted")
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
	defer st.server.Close()

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

// TestIntegrationWalletBlankEncrypt tries to encrypt and unlock the wallet
// through the api using a blank encryption key - meaning that the wallet seed
// returned by the encryption call can be used as the encryption key.
func TestIntegrationWalletBlankEncrypt(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a server object without encrypting or unlocking the wallet.
	testdir := build.TempDir("api", "TestIntegrationWalletBlankEncrypt")
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
	defer st.server.Close()

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
	if !w.Unlocked() {
		t.Error("wallet is not unlocked")
	}
}

// TestIntegrationWalletGETSiacoins probes the GET call to /wallet when the
// siacoin balance is being manipulated.
func TestIntegrationWalletGETSiacoins(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationWalletGETSiacoins")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
	if wg.UnconfirmedOutgoingSiacoins.Cmp(types.NewCurrency64(0)) != 0 {
		t.Error("there should not be unconfirmed outgoing siacoins")
	}
	if wg.UnconfirmedIncomingSiacoins.Cmp(types.NewCurrency64(0)) != 0 {
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
	if wg.UnconfirmedOutgoingSiacoins.Cmp(types.NewCurrency64(0)) <= 0 {
		t.Error("there should be unconfirmed outgoing siacoins")
	}
	if wg.UnconfirmedIncomingSiacoins.Cmp(types.NewCurrency64(0)) <= 0 {
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
	if wg.UnconfirmedOutgoingSiacoins.Cmp(types.NewCurrency64(0)) != 0 {
		t.Error("there should not be unconfirmed outgoing siacoins")
	}
	if wg.UnconfirmedIncomingSiacoins.Cmp(types.NewCurrency64(0)) != 0 {
		t.Error("there should not be unconfirmed incoming siacoins")
	}
}

// TestIntegrationWalletTransactionGETid queries the /wallet/transaction/$(id)
// api call.
func TestIntegrationWalletTransactionGETid(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationWalletTransactionGETid")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
	// /wallet/transaction/$(id)
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
}

// Tests that the /wallet/backup call checks for relative paths.
func TestWalletRelativePathErrorBackup(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestWalletRelativePathErrorBackup")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
	st, err := createServerTester("TestWalletRelativePathError033x")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
	st, err := createServerTester("TestWalletRelativePathErrorSiag")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
