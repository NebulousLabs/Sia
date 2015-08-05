package wallet

import (
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
)

// TestIntegrationEncrypted checks the encrypted status of the wallet.
func TestIntegrationEncrypted(t *testing.T) {
	dir := build.TempDir(modules.WalletDir, "TestEncrypted")
	g, err := gateway.New(":0", filepath.Join(dir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, filepath.Join(dir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal(err)
	}
	w0, err := New(cs, tp, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet determines that it is not encrypted.
	if w0.Encrypted() {
		t.Error("wallet is reporting that it has been encrypted")
	}
	w0.Close()

	// Create a second wallet using the same directory.
	w1, err := New(cs, tp, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	if w1.Encrypted() {
		t.Error("wallet is reporting that it has been encrypted when no such action has occured")
	}

	// Create an unlock key and unlock the wallet - this will encrypt the
	// wallet using the master key.
	var masterKey crypto.TwofishKey
	_, err = rand.Read(masterKey[:])
	if err != nil {
		t.Fatal(err)
	}
	err = w1.Unlock(masterKey)
	if err != nil {
		t.Fatal(err)
	}
	if !w1.Encrypted() {
		t.Error("Wallet is not returning as encrypted after bing unlocked.")
	}
	err = w1.Unlock(masterKey)
	if err != errAlreadyUnlocked {
		t.Error(err)
	}
	w1.Close()

	// Create a wallet and see if it loads the encrypted file.
	w2, err := New(cs, tp, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	if !w2.Encrypted() {
		t.Error("Wallet is reporting as not encrypted after loading")
	}
	// Unlock with the wrong key.
	err = w2.Unlock(crypto.TwofishKey{})
	if err == nil {
		t.Error(err)
	}
	err = w2.Unlock(masterKey)
	if err != nil {
		t.Error(err)
	}
	// Unlock twice, which should return an error.
	err = w2.Unlock(masterKey)
	if err == nil {
		t.Error(err)
	}
}
