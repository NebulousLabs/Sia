package api

import (
	"net/url"
	"path/filepath"
	"strconv"
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
	APIAddr := ":" + strconv.Itoa(APIPort)
	APIPort++
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal("Failed to create gateway:", err)
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal("Failed to create consensus set:", err)
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal("Failed to create tpool:", err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal("Failed to create wallet:", err)
	}
	srv, err := NewServer(APIAddr, cs, g, nil, nil, nil, nil, tp, w, nil)
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
	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			t.Fatal("API server quit:", listenErr)
		}
	}()

	var wg WalletGET
	err = st.getAPI("/wallet", &wg)
	if err != nil {
		t.Fatal(err)
	}
	if wg.Encrypted {
		t.Error("Wallet has never been unlocked")
	}
	if wg.Unlocked {
		t.Error("Wallet has never been unlocked")
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

	// Check the intial wallet is encrypted, unlocked, and has the siacoins
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
	var wap WalletAddressPOST
	err = st.postAPI("/wallet/address", url.Values{}, &wap)
	if err != nil {
		t.Fatal(err)
	}
	sendSiacoinsValues := url.Values{}
	sendSiacoinsValues.Set("amount", "1234")
	sendSiacoinsValues.Add("destination", wap.Address.String())
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
