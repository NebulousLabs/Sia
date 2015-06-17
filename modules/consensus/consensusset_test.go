package consensus

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
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
	gateway modules.Gateway
	miner   modules.Miner
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	cs *State

	persistDir string

	csUpdateChan     <-chan struct{}
	minerUpdateChan  <-chan struct{}
	tpoolUpdateChan  <-chan struct{}
	walletUpdateChan <-chan struct{}
}

// csUpdateWait blocks until an update to the consensus set has propagated to
// all modules.
func (cst *consensusSetTester) csUpdateWait() {
	<-cst.csUpdateChan
	cst.tpUpdateWait()
}

// tpUpdateWait blocks until an update to the transaction pool has propagated
// to all modules.
func (cst *consensusSetTester) tpUpdateWait() {
	<-cst.tpoolUpdateChan
	<-cst.minerUpdateChan
	<-cst.walletUpdateChan
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
	m, err := miner.New(cs, tp, w)
	if err != nil {
		return nil, err
	}

	// Assemble all objects into a consensusSetTester.
	cst := &consensusSetTester{
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		cs: cs,

		persistDir: testdir,

		csUpdateChan:     cs.ConsensusSetNotify(),
		minerUpdateChan:  m.MinerNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		walletUpdateChan: w.WalletNotify(),
	}
	cst.csUpdateWait()

	// Mine until the wallet has money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
		cst.csUpdateWait()
	}
	return cst, nil
}

// MineDoSBlock will create a dos block and perform nonce grinding.
func (cst *consensusSetTester) MineDoSBlock() (types.Block, error) {
	// Create a transaction that is funded but the funds are never spent. This
	// transaction is invalid in a way that triggers the DoS block detection.
	id, err := cst.wallet.RegisterTransaction(types.Transaction{})
	if err != nil {
		return types.Block{}, err
	}
	_, err = cst.wallet.FundTransaction(id, types.NewCurrency64(50))
	if err != nil {
		return types.Block{}, err
	}
	cst.tpUpdateWait()
	txn, err := cst.wallet.SignTransaction(id, true) // true indicates that the whole transaction should be signed.
	if err != nil {
		return types.Block{}, err
	}

	// Get a block, insert the transaction, and submit the block.
	block, _, target := cst.miner.BlockForWork()
	block.Transactions = append(block.Transactions, txn)
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	return solvedBlock, nil
}

// checkCurrentPath looks at the blocks in the current path and verifies it
// makes sense in the context of the rest of the consensus set.
func (cst *consensusSetTester) checkCurrentPath() error {
	lockID := cst.cs.mu.RLock()
	defer cst.cs.mu.RUnlock(lockID)

	// Check the block at each height.
	currentNode := cst.cs.currentBlockNode()
	for i := cst.cs.height(); i != 0; i-- {
		// Block should be in the block map.
		_, exists := cst.cs.blockMap[currentNode.block.ID()]
		if !exists {
			return errors.New("current inconsistent - a block is missing from the block map")
		}

		// The current node id should match the id listed in the current path.
		if currentNode.block.ID() != cst.cs.currentPath[i] {
			return errors.New("current path is inconsistent with the block history.")
		}

		// Check the height listed in the node is correct.
		if currentNode.height != currentNode.parent.height+1 {
			return errors.New("node heights are misaligned")
		}
		if currentNode.height != i {
			return errors.New("node height mismatched with its location in the current path")
		}

		// Check that the block points to the correct parent.
		if i != 0 {
			if currentNode.block.ParentID != currentNode.parent.block.ID() {
				return errors.New("parent-child structure misaligned")
			}
		}
		currentNode = currentNode.parent
	}
	return nil
}

// checkDiffStructure reverts and then reapplies all diffs in the blockchain
// history and verifies that the result is the same as computed initially.
func (cst *consensusSetTester) checkDiffStructure() error {
	lockID := cst.cs.mu.RLock()
	// Revert and reapply all diffs in place.
	initialSum := cst.cs.consensusSetHash()
	currentNode := cst.cs.currentBlockNode()
	firstNode := cst.cs.blockMap[cst.cs.currentPath[0]]
	_, _, err := cst.cs.forkBlockchain(firstNode)
	if err != nil {
		cst.cs.mu.RUnlock(lockID)
		return err
	}
	_, _, err = cst.cs.forkBlockchain(currentNode)
	if err != nil {
		cst.cs.mu.RUnlock(lockID)
		return err
	}
	if initialSum != cst.cs.consensusSetHash() {
		cst.cs.mu.RUnlock(lockID)
		return errors.New("reverting and reapplying resulted in mismatched consensus set hash")
	}
	cst.cs.mu.RUnlock(lockID)

	// Try from a clean slate. Make a fresh blockchain and then apply all of
	// the diffs and check for a consensus set match.
	cs, err := New(cst.gateway, filepath.Join(cst.persistDir, "checkDiffStructure"))
	if err != nil {
		return err
	}

	lockID = cst.cs.mu.RLock()
	defer cst.cs.mu.RUnlock(lockID)
	for i := 1; i < len(cst.cs.currentPath); i++ {
		node := cst.cs.blockMap[cst.cs.currentPath[i]]
		cs.blockMap[node.block.ID()] = node
		_, _, err = cs.forkBlockchain(node)
		if err != nil {
			return err
		}
	}
	if initialSum != cs.consensusSetHash() {
		return errors.New("clean slate approach failed")
	}
	return nil
}

// checkCurrency verifies that the amount of currency in the system matches the
// amount of currency that is supposed to be in the system.
func (cst *consensusSetTester) checkCurrency() error {
	lockID := cst.cs.mu.RLock()
	defer cst.cs.mu.RUnlock(lockID)

	// Check that there are 10k siafunds.
	totalSiafunds := types.NewCurrency64(0)
	for _, sfo := range cst.cs.siafundOutputs {
		totalSiafunds = totalSiafunds.Add(sfo.Value)
	}
	if totalSiafunds.Cmp(types.NewCurrency64(types.SiafundCount)) != 0 {
		return errors.New("incorrect number of siafunds in the consensus set")
	}

	// Check that there are the expected number of siacoins.
	expectedSiacoins := types.NewCurrency64(0)
	for i := types.BlockHeight(0); i <= cst.cs.height(); i++ {
		expectedSiacoins = expectedSiacoins.Add(types.CalculateCoinbase(i))
	}
	totalSiacoins := cst.cs.siafundPool
	for _, sco := range cst.cs.siacoinOutputs {
		totalSiacoins = totalSiacoins.Add(sco.Value)
	}
	for _, fc := range cst.cs.fileContracts {
		totalSiacoins = totalSiacoins.Add(fc.Payout)
	}
	for height, dsoMap := range cst.cs.delayedSiacoinOutputs {
		if height+types.MaturityDelay > cst.cs.Height() {
			for _, dso := range dsoMap {
				totalSiacoins = totalSiacoins.Add(dso.Value)
			}
		}
	}
	if expectedSiacoins.Cmp(totalSiacoins) != 0 {
		return errors.New("incorrect number of siacoins in the consensus set")
	}
	return nil
}

// checkConsistency runs all of the consensus set tester's consistency checks.
func (cst *consensusSetTester) checkConsistency() error {
	err := cst.checkCurrentPath()
	if err != nil {
		return err
	}
	err = cst.checkDiffStructure()
	if err != nil {
		return err
	}
	err = cst.checkCurrency()
	if err != nil {
		return err
	}
	return nil
}

// TestNilInputs tries to create new consensus set modules using nil inputs.
func TestNilInputs(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, "TestNilInputs")
	_, err := New(nil, testdir)
	if err != ErrNilGateway {
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
