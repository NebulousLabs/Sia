package consensus

import (
	"bytes"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// A consensusSetTester is the helper object for consensus set testing,
// including helper modules and methods for controlling synchronization between
// the tester and the modules.
type consensusSetTester struct {
	gateway   modules.Gateway
	miner     modules.Miner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	cs *ConsensusSet

	persistDir string
}

// randAddress returns a random address that is not spendable.
func randAddress() types.UnlockHash {
	var uh types.UnlockHash
	_, err := rand.Read(uh[:])
	if err != nil {
		panic(err)
	}
	return uh
}

// randFile returns a bytes.Reader that is equivalent to a random file of size
// 'filesize'.
func randFile(filesize uint64) *bytes.Reader {
	fileBytes := make([]byte, filesize)
	_, err := rand.Read(fileBytes)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(fileBytes)
}

// addSiafunds makes a transaction that moves all of the testing genesis
// siafunds into the wallet.
func (cst *consensusSetTester) addSiafunds() {
	// Get an address to receive the siafunds.
	uc, err := cst.wallet.NextAddress()
	if err != nil {
		panic(err)
	}

	// Create the transaction that sends the anyone-can-spend siafund output to
	// the wallet address (output only available during testing).
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{{
			ParentID:         cst.cs.blockRoot.Block.Transactions[0].SiafundOutputID(2),
			UnlockConditions: types.UnlockConditions{},
		}},
		SiafundOutputs: []types.SiafundOutput{{
			Value:      types.NewCurrency64(1e3),
			UnlockHash: uc.UnlockHash(),
		}},
	}

	// Mine the transaction into the blockchain.
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		panic(err)
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		panic(err)
	}

	// Check that the siafunds made it to the wallet.
	_, siafundBalance, _ := cst.wallet.ConfirmedBalance()
	if siafundBalance.Cmp(types.NewCurrency64(1e3)) != 0 {
		panic("wallet does not have the siafunds")
	}
}

// mineCoins mines blocks until there are siacoins in the wallet.
func (cst *consensusSetTester) mineSiacoins() {
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := cst.miner.FindBlock()
		err := cst.cs.AcceptBlock(b)
		if err != nil {
			panic(err)
		}
	}
}

// createConsensusSetTester creates a consensusSetTester that's ready for use.
func createConsensusSetTester(name string) (*consensusSetTester, error) {
	testdir := build.TempDir(modules.ConsensusDir, name)

	// Create modules.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all objects into a consensusSetTester.
	cst := &consensusSetTester{
		gateway:   g,
		miner:     m,
		tpool:     tp,
		wallet:    w,
		walletKey: key,

		cs: cs,

		persistDir: testdir,
	}
	cst.addSiafunds()
	cst.mineSiacoins()

	return cst, nil
}

// closeCst safely closes the consensus set tester. 'close' is a builtin
func (cst *consensusSetTester) closeCst() error {
	return cst.gateway.Close()
}

// TestNilInputs tries to create new consensus set modules using nil inputs.
func TestNilInputs(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, "TestNilInputs")
	_, err := New(nil, testdir)
	if err != errNilGateway {
		t.Fatal(err)
	}
}

// TestClosing tries to close a consenuss set.
func TestDatabaseClosing(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, "TestClosing")

	// Create the gateway.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, testdir)
	if err != nil {
		t.Fatal(err)
	}
	err = cs.Close()
	if err != nil {
		t.Error(err)
	}
}
