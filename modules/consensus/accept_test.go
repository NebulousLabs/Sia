package consensus

import (
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationDoSBlockHandling checks that saved bad blocks are correctly ignored.
func TestIntegrationDoSBlockHandling(t *testing.T) {
	// TestIntegrationDoSBlockHandling catches a wide array of simple errors,
	// and therefore is included in the short tests despite being somewhat
	// computationally expensive.
	cst, err := createConsensusSetTester("TestIntegrationDoSBlockHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Mine a block that is valid except for containing a buried invalid
	// transaction. The transaction has more siacoin inputs than outputs.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(types.NewCurrency64(50))
	if err != nil {
		t.Fatal(err)
	}
	txnSet, err := txnBuilder.Sign(true) // true sets the 'wholeTransaction' flag
	if err != nil {
		t.Fatal(err)
	}

	// Mine and submit the invalid block to the consensus set. The first time
	// around, the complaint should be about the rule-breaking transaction.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.Transactions = append(block.Transactions, txnSet...)
	dosBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.AcceptBlock(dosBlock)
	if err != errSiacoinInputOutputMismatch {
		t.Fatalf("expected %v, got %v", errSiacoinInputOutputMismatch, err)
	}

	// Submit the same block a second time. The complaint should be that the
	// block is already known to be invalid.
	err = cst.cs.AcceptBlock(dosBlock)
	if err != errDoSBlock {
		t.Fatalf("expected %v, got %v", errDoSBlock, err)
	}
}

// TestBlockKnownHandling submits known blocks to the consensus set.
func TestBlockKnownHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBlockKnownHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Get a block destined to be stale.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	staleBlock, _ := cst.miner.SolveBlock(block, target)

	// Add two new blocks to the consensus set to block the stale block.
	block1, err := cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	block2, err := cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Submit the stale block.
	err = cst.cs.acceptBlock(staleBlock)
	if err != nil && err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}

	// Submit block1 and block2 again, looking for a 'BlockKnown' error.
	err = cst.cs.acceptBlock(block1)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}
	err = cst.cs.acceptBlock(block2)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}
	err = cst.cs.acceptBlock(staleBlock)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}

	// Try the genesis block edge case.
	id, err := cst.cs.dbGetPath(0)
	if err != nil {
		t.Fatal(err)
	}
	genesisBlock, err := cst.cs.dbGetBlockMap(id)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.cs.acceptBlock(genesisBlock.Block)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}
}

// TestOrphanHandling passes an orphan block to the consensus set.
func TestOrphanHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestOrphanHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// The empty block is an orphan.
	orphan := types.Block{}
	err = cst.cs.acceptBlock(orphan)
	if err != errOrphan {
		t.Fatalf("expected %v, got %v", errOrphan, err)
	}
	err = cst.cs.acceptBlock(orphan)
	if err != errOrphan {
		t.Fatalf("expected %v, got %v", errOrphan, err)
	}
}

// TestMissedTarget submits a block that does not meet the required target.
func TestMissedTarget(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMissedTarget")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Mine a block that doesn't meet the target.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	for block.CheckTarget(target) && block.Nonce[0] != 255 {
		block.Nonce[0]++
	}
	if block.CheckTarget(target) {
		t.Fatal("unable to find a failing target")
	}
	err = cst.cs.acceptBlock(block)
	if err != errMissedTarget {
		t.Fatalf("expected %v, got %v", errMissedTarget, err)
	}
}

// testLargeBlock creates a block that is too large to be accepted by the state
// and checks that it actually gets rejected.
func TestLargeBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestLargeBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a transaction that puts the block over the size limit.
	bigData := make([]byte, types.BlockSizeLimit)
	txn := types.Transaction{
		ArbitraryData: [][]byte{bigData},
	}

	// Fetch a block and add the transaction, then submit the block.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.Transactions = append(block.Transactions, txn)
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(solvedBlock)
	if err != errLargeBlock {
		t.Fatalf("expected %v, got %v", errLargeBlock, err)
	}
}

// TestEarlyBlockTimestampHandling checks that blocks with early timestamps are
// handled appropriately.
func TestEarlyBlockTimestampHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBlockTimestampHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block with a too early timestamp - block should be rejected
	// outright.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.Timestamp = 0
	earlyBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(earlyBlock)
	if err != errEarlyTimestamp {
		t.Fatalf("expected %v, got %v", errEarlyTimestamp, err)
	}
}

// TestExtremeFutureTimestampHandling checks that blocks with extreme future
// timestamps handled correclty.
func TestExtremeFutureTimestampHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestExtremeFutureTimestampHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Submit a block with a timestamp in the extreme future.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.Timestamp = types.CurrentTimestamp() + 2 + types.ExtremeFutureThreshold
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(solvedBlock)
	if err != errExtremeFutureTimestamp {
		t.Fatalf("expected %v, got %v", errExtremeFutureTimestamp, err)
	}

	// Check that after waiting until the block is no longer in the future, the
	// block still has not been added to the consensus set (prove that the
	// block was correctly discarded).
	time.Sleep(time.Second * time.Duration(3+types.ExtremeFutureThreshold))
	lockID := cst.cs.mu.RLock()
	defer cst.cs.mu.RUnlock(lockID)
	_, err = cst.cs.dbGetBlockMap(solvedBlock.ID())
	if err != errNilItem {
		t.Error("extreme future block made it into the consensus set after waiting")
	}
}

// TestMinerPayoutHandling checks that blocks with incorrect payouts are
// rejected.
func TestMinerPayoutHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMinerPayoutHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block with the wrong miner payout structure - testing can be
	// light here because there is heavier testing in the 'types' package,
	// where the logic is defined.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.MinerPayouts = append(block.MinerPayouts, types.SiacoinOutput{Value: types.NewCurrency64(1)})
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(solvedBlock)
	if err != errBadMinerPayouts {
		t.Fatalf("expected %v, got %v", errBadMinerPayouts, err)
	}
}

// testFutureTimestampHandling checks that blocks in the future (but not
// extreme future) are handled correctly.
func (cst *consensusSetTester) testFutureTimestampHandling() error {
	// Submit a block with a timestamp in the future, but not the extreme
	// future.
	block, _, target, err := cst.miner.BlockForWork()
	if err != nil {
		return err
	}
	block.Timestamp = types.CurrentTimestamp() + 2 + types.FutureThreshold
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(solvedBlock)
	if err != errFutureTimestamp {
		return fmt.Errorf("expected %v, got %v", errFutureTimestamp, err)
	}

	// Check that after waiting until the block is no longer too far in the
	// future, the block gets added to the consensus set.
	time.Sleep(time.Second * 3) // 3 seconds, as the block was originally 2 seconds too far into the future.
	lockID := cst.cs.mu.RLock()
	defer cst.cs.mu.RUnlock(lockID)
	_, err = cst.cs.dbGetBlockMap(solvedBlock.ID())
	if err == errNilItem {
		return errors.New("future block was not added to the consensus set after waiting the appropriate amount of time")
	}
	return nil
}

// TestFutureTimestampHandling creates a consensus set tester and uses it to
// call testFutureTimestampHandling.
func TestFutureTimestampHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestFutureTimestampHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	err = cst.testFutureTimestampHandling()
	if err != nil {
		t.Error(err)
	}
}

/*
// TestInconsistentCheck submits a block on a consensus set that is
// inconsistent, attempting to trigger a panic.
func TestInconsistentCheck(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestInconsistentCheck")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Corrupt the consensus set.
	var sfod types.SiafundOutputID
	var sfo types.SiafundOutput
	cst.cs.db.forEachSiafundOutputs(func(id types.SiafundOutputID, output types.SiafundOutput) {
		sfod = id
		sfo = output
	})
	sfo.Value = sfo.Value.Add(types.NewCurrency64(1))
	cst.cs.db.rmSiafundOutputs(sfod)
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		return addSiafundOutput(tx, sfod, sfo)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine and submit a block, triggering the inconsistency check.
	defer func() {
		r := recover()
		if r != errSiafundMiscount {
			t.Fatalf("expected %v, got %v", errSiafundMiscount, err)
		}
	}()
	cst.miner.AddBlock()
}
*/

// testSpendSiafundsBlock mines a block with a transaction spending siafunds
// and adds it to the consensus set.
func (cst *consensusSetTester) testSpendSiafundsBlock() error {
	// Create a destination for the siafunds.
	var destAddr types.UnlockHash
	_, err := rand.Read(destAddr[:])
	if err != nil {
		return err
	}

	// Find the siafund output that is 'anyone can spend' (output exists only
	// in the testing setup).
	var srcID types.SiafundOutputID
	var srcValue types.Currency
	anyoneSpends := types.UnlockConditions{}.UnlockHash()
	cst.cs.db.forEachSiafundOutputs(func(id types.SiafundOutputID, sfo types.SiafundOutput) {
		if sfo.UnlockHash == anyoneSpends {
			srcID = id
			srcValue = sfo.Value
		}
	})

	// Create a transaction that spends siafunds.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{{
			ParentID:         srcID,
			UnlockConditions: types.UnlockConditions{},
		}},
		SiafundOutputs: []types.SiafundOutput{
			{
				Value:      srcValue.Sub(types.NewCurrency64(1)),
				UnlockHash: types.UnlockConditions{}.UnlockHash(),
			},
			{
				Value:      types.NewCurrency64(1),
				UnlockHash: destAddr,
			},
		},
	}
	sfoid0 := txn.SiafundOutputID(0)
	sfoid1 := txn.SiafundOutputID(1)
	cst.tpool.AcceptTransactionSet([]types.Transaction{txn})

	// Mine a block containing the txn.
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}

	// Check that the input got consumed, and that the outputs got created.
	exists := cst.cs.db.inSiafundOutputs(srcID)
	if exists {
		return errors.New("siafund output was not properly consumed")
	}
	exists = cst.cs.db.inSiafundOutputs(sfoid0)
	if !exists {
		return errors.New("siafund output was not properly created")
	}
	sfo, err := cst.cs.dbGetSiafundOutput(sfoid0)
	if err != nil {
		return err
	}
	if sfo.Value.Cmp(srcValue.Sub(types.NewCurrency64(1))) != 0 {
		return errors.New("created siafund has wrong value")
	}
	if sfo.UnlockHash != anyoneSpends {
		return errors.New("siafund output sent to wrong unlock hash")
	}
	exists = cst.cs.db.inSiafundOutputs(sfoid1)
	if !exists {
		return errors.New("second siafund output was not properly created")
	}
	sfo, err = cst.cs.dbGetSiafundOutput(sfoid1)
	if err != nil {
		return err
	}
	if sfo.Value.Cmp(types.NewCurrency64(1)) != 0 {
		return errors.New("second siafund output has wrong value")
	}
	if sfo.UnlockHash != destAddr {
		return errors.New("second siafund output sent to wrong addr")
	}

	// Put a file contract into the blockchain that will add values to siafund
	// outputs.
	var siafundPool types.Currency
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		siafundPool = getSiafundPool(tx)
		return nil
	})
	if err != nil {
		panic(err)
	}
	oldSiafundPool := siafundPool
	payout := types.NewCurrency64(400e6)
	fc := types.FileContract{
		WindowStart: cst.cs.dbBlockHeight() + 2,
		WindowEnd:   cst.cs.dbBlockHeight() + 4,
		Payout:      payout,
	}
	outputSize := payout.Sub(types.Tax(cst.cs.dbBlockHeight(), fc.Payout))
	fc.ValidProofOutputs = []types.SiacoinOutput{{Value: outputSize}}
	fc.MissedProofOutputs = []types.SiacoinOutput{{Value: outputSize}}

	// Create and fund a transaction with a file contract.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(payout)
	if err != nil {
		return err
	}
	txnBuilder.AddFileContract(fc)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return err
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		siafundPool = getSiafundPool(tx)
		return nil
	})
	if err != nil {
		panic(err)
	}
	if siafundPool.Cmp(types.NewCurrency64(15600e3).Add(oldSiafundPool)) != 0 {
		return errors.New("siafund pool did not update correctly")
	}

	// Create a transaction that spends siafunds.
	var claimDest types.UnlockHash
	_, err = rand.Read(claimDest[:])
	if err != nil {
		return err
	}
	var srcClaimStart types.Currency
	cst.cs.db.forEachSiafundOutputs(func(id types.SiafundOutputID, sfo types.SiafundOutput) {
		if sfo.UnlockHash == anyoneSpends {
			srcID = id
			srcValue = sfo.Value
			srcClaimStart = sfo.ClaimStart
		}
	})
	txn = types.Transaction{
		SiafundInputs: []types.SiafundInput{{
			ParentID:         srcID,
			UnlockConditions: types.UnlockConditions{},
			ClaimUnlockHash:  claimDest,
		}},
		SiafundOutputs: []types.SiafundOutput{
			{
				Value:      srcValue.Sub(types.NewCurrency64(1)),
				UnlockHash: types.UnlockConditions{}.UnlockHash(),
			},
			{
				Value:      types.NewCurrency64(1),
				UnlockHash: destAddr,
			},
		},
	}
	sfoid1 = txn.SiafundOutputID(1)
	cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}

	// Find the siafund output and check that it has the expected number of
	// siafunds.
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		siafundPool = getSiafundPool(tx)
		return nil
	})
	if err != nil {
		panic(err)
	}
	found := false
	expectedBalance := siafundPool.Sub(srcClaimStart).Div(types.NewCurrency64(10e3)).Mul(srcValue)
	cst.cs.db.forEachDelayedSiacoinOutputsHeight(cst.cs.dbBlockHeight()+types.MaturityDelay, func(id types.SiacoinOutputID, output types.SiacoinOutput) {
		if output.UnlockHash == claimDest {
			found = true
			if output.Value.Cmp(expectedBalance) != 0 {
				// err is scoped outside this func
				err = errors.New("siafund output has the wrong balance")
			}
		}
	})
	if err != nil {
		return err
	}
	if !found {
		return errors.New("could not find siafund claim output")
	}

	return nil
}

// TestSpendSiafundsBlock creates a consensus set tester and uses it to call
// testSpendSiafundsBlock.
func TestSpendSiafundsBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestSpendSiafundsBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// COMPATv0.4.0
	//
	// Mine enough blocks to get above the file contract hardfork threshold
	// (10).
	for i := 0; i < 10; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = cst.testSpendSiafundsBlock()
	if err != nil {
		t.Error(err)
	}
}

// testPaymentChannelBlocks submits blocks to set up, use, and close a payment
// channel.
func (cst *consensusSetTester) testPaymentChannelBlocks() error {
	// The current method of doing payment channels is gimped because public
	// keys do not have timelocks. We will be hardforking to include timelocks
	// in public keys in 0.4.0, but in the meantime we need an alternate
	// method.

	// Gimped payment channels: 2-of-2 multisig where one key is controlled by
	// the funding entity, and one key is controlled by the receiving entity. An
	// address is created containing both keys, and then the funding entity
	// creates, but does not sign, a transaction sending coins to the channel
	// address. A second transaction is created that sends all the coins in the
	// funding output back to the funding entity. The receiving entity signs the
	// transaction with a timelocked signature. The funding entity will get the
	// refund after T blocks as long as the output is not double spent. The
	// funding entity then signs the first transaction and opens the channel.
	//
	// Creating the channel:
	//	1. Create a 2-of-2 unlock conditions, one key held by each entity.
	//	2. Funding entity creates, but does not sign, a transaction sending
	//		money to the payment channel address. (txn A)
	//	3. Funding entity creates and signs a transaction spending the output
	//		created in txn A that sends all the money back as a refund. (txn B)
	//	4. Receiving entity signs txn B with a timelocked signature, so that the
	//		funding entity cannot get the refund for several days. The funding entity
	//		is given a fully signed and eventually-spendable txn B.
	//	5. The funding entity signs and broadcasts txn A.
	//
	// Using the channel:
	//	Each the receiving entity and the funding entity keeps a record of how
	//	much has been sent down the unclosed channel, and watches the
	//	blockchain for a channel closing transaction. To send more money down
	//	the channel, the funding entity creates and signs a transaction sending
	//	X+y coins to the receiving entity from the channel address. The
	//	transaction is sent to the receiving entity, who will keep it and
	//	potentially sign and broadcast it later. The funding entity will only
	//	send money down the channel if 'work' or some other sort of event has
	//	completed that indicates the receiving entity should get more money.
	//
	// Closing the channel:
	//	The receiving entity will sign the transaction that pays them the most
	//	money and then broadcast that transaction. This will spend the output
	//	and close the channel, invalidating txn B and preventing any future
	//	transactions from being made over the channel. The channel must be
	//	closed before the timelock expires on the second signature in txn B,
	//	otherwise the funding entity will be able to get a full refund.
	//
	//	The funding entity should be waiting until either the receiving entity
	//	closes the channel or the timelock expires. If the receiving entity
	//	closes the channel, all is good. If not, then the funding entity can
	//	close the channel and get a full refund.

	// Create a 2-of-2 unlock conditions, 1 key for each the sender and the
	// receiver in the payment channel.
	sk1, pk1, err := crypto.GenerateSignatureKeys() // Funding entity.
	if err != nil {
		return err
	}
	sk2, pk2, err := crypto.GenerateSignatureKeys() // Receiving entity.
	if err != nil {
		return err
	}
	uc := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{
			{
				Algorithm: types.SignatureEd25519,
				Key:       pk1[:],
			},
			{
				Algorithm: types.SignatureEd25519,
				Key:       pk2[:],
			},
		},
		SignaturesRequired: 2,
	}
	channelAddress := uc.UnlockHash()

	// Funding entity creates but does not sign a transaction that funds the
	// channel address. Because the wallet is not very flexible, the channel
	// txn needs to be fully custom. To get a custom txn, manually create an
	// address and then use the wallet to fund that address.
	channelSize := types.NewCurrency64(10e3)
	channelFundingSK, channelFundingPK, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return err
	}
	channelFundingUC := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       channelFundingPK[:],
		}},
		SignaturesRequired: 1,
	}
	channelFundingAddr := channelFundingUC.UnlockHash()
	fundTxnBuilder := cst.wallet.StartTransaction()
	if err != nil {
		return err
	}
	err = fundTxnBuilder.FundSiacoins(channelSize)
	if err != nil {
		return err
	}
	scoFundIndex := fundTxnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: channelSize, UnlockHash: channelFundingAddr})
	fundTxnSet, err := fundTxnBuilder.Sign(true)
	if err != nil {
		return err
	}
	fundOutputID := fundTxnSet[len(fundTxnSet)-1].SiacoinOutputID(int(scoFundIndex))
	channelTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         fundOutputID,
			UnlockConditions: channelFundingUC,
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      channelSize,
			UnlockHash: channelAddress,
		}},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(fundOutputID),
			PublicKeyIndex: 0,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		}},
	}

	// Funding entity creates and signs a transaction that spends the full
	// channel output.
	channelOutputID := channelTxn.SiacoinOutputID(0)
	refundUC, err := cst.wallet.NextAddress()
	refundAddr := refundUC.UnlockHash()
	if err != nil {
		return err
	}
	refundTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         channelOutputID,
			UnlockConditions: uc,
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      channelSize,
			UnlockHash: refundAddr,
		}},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(channelOutputID),
			PublicKeyIndex: 0,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		}},
	}
	sigHash := refundTxn.SigHash(0)
	cryptoSig1, err := crypto.SignHash(sigHash, sk1)
	if err != nil {
		return err
	}
	refundTxn.TransactionSignatures[0].Signature = cryptoSig1[:]

	// Receiving entity signs the transaction that spends the full channel
	// output, but with a timelock.
	refundTxn.TransactionSignatures = append(refundTxn.TransactionSignatures, types.TransactionSignature{
		ParentID:       crypto.Hash(channelOutputID),
		PublicKeyIndex: 1,
		Timelock:       cst.cs.dbBlockHeight() + 2,
		CoveredFields:  types.CoveredFields{WholeTransaction: true},
	})
	sigHash = refundTxn.SigHash(1)
	cryptoSig2, err := crypto.SignHash(sigHash, sk2)
	if err != nil {
		return err
	}
	refundTxn.TransactionSignatures[1].Signature = cryptoSig2[:]

	// Funding entity will now sign and broadcast the funding transaction.
	sigHash = channelTxn.SigHash(0)
	cryptoSig0, err := crypto.SignHash(sigHash, channelFundingSK)
	if err != nil {
		return err
	}
	channelTxn.TransactionSignatures[0].Signature = cryptoSig0[:]
	err = cst.tpool.AcceptTransactionSet(append(fundTxnSet, channelTxn))
	if err != nil {
		return err
	}
	// Put the txn in a block.
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}

	// Try to submit the refund transaction before the timelock has expired.
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{refundTxn})
	if err != types.ErrPrematureSignature {
		return err
	}

	// Create a transaction that has partially used the channel, and submit it
	// to the blockchain to close the channel.
	closeTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         channelOutputID,
			UnlockConditions: uc,
		}},
		SiacoinOutputs: []types.SiacoinOutput{
			{
				Value:      channelSize.Sub(types.NewCurrency64(5)),
				UnlockHash: refundAddr,
			},
			{
				Value: types.NewCurrency64(5),
			},
		},
		TransactionSignatures: []types.TransactionSignature{
			{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			},
			{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 1,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			},
		},
	}
	sigHash = closeTxn.SigHash(0)
	cryptoSig3, err := crypto.SignHash(sigHash, sk1)
	if err != nil {
		return err
	}
	closeTxn.TransactionSignatures[0].Signature = cryptoSig3[:]
	sigHash = closeTxn.SigHash(1)
	cryptoSig4, err := crypto.SignHash(sigHash, sk2)
	if err != nil {
		return err
	}
	closeTxn.TransactionSignatures[1].Signature = cryptoSig4[:]
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{closeTxn})
	if err != nil {
		return err
	}

	// Mine the block with the transaction.
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}
	closeRefundID := closeTxn.SiacoinOutputID(0)
	closePaymentID := closeTxn.SiacoinOutputID(1)
	exists := cst.cs.db.inSiacoinOutputs(closeRefundID)
	if !exists {
		return errors.New("close txn refund output doesn't exist")
	}
	exists = cst.cs.db.inSiacoinOutputs(closePaymentID)
	if !exists {
		return errors.New("close txn payment output doesn't exist")
	}

	// Create a payment channel where the receiving entity never responds to
	// the initial transaction.
	{
		// Funding entity creates but does not sign a transaction that funds the
		// channel address. Because the wallet is not very flexible, the channel
		// txn needs to be fully custom. To get a custom txn, manually create an
		// address and then use the wallet to fund that address.
		channelSize := types.NewCurrency64(10e3)
		channelFundingSK, channelFundingPK, err := crypto.GenerateSignatureKeys()
		if err != nil {
			return err
		}
		channelFundingUC := types.UnlockConditions{
			PublicKeys: []types.SiaPublicKey{{
				Algorithm: types.SignatureEd25519,
				Key:       channelFundingPK[:],
			}},
			SignaturesRequired: 1,
		}
		channelFundingAddr := channelFundingUC.UnlockHash()
		fundTxnBuilder := cst.wallet.StartTransaction()
		err = fundTxnBuilder.FundSiacoins(channelSize)
		if err != nil {
			return err
		}
		scoFundIndex := fundTxnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: channelSize, UnlockHash: channelFundingAddr})
		fundTxnSet, err := fundTxnBuilder.Sign(true)
		if err != nil {
			return err
		}
		fundOutputID := fundTxnSet[len(fundTxnSet)-1].SiacoinOutputID(int(scoFundIndex))
		channelTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         fundOutputID,
				UnlockConditions: channelFundingUC,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: channelAddress,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(fundOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}

		// Funding entity creates and signs a transaction that spends the full
		// channel output.
		channelOutputID := channelTxn.SiacoinOutputID(0)
		refundUC, err := cst.wallet.NextAddress()
		refundAddr := refundUC.UnlockHash()
		if err != nil {
			return err
		}
		refundTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         channelOutputID,
				UnlockConditions: uc,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: refundAddr,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}
		sigHash := refundTxn.SigHash(0)
		cryptoSig1, err := crypto.SignHash(sigHash, sk1)
		if err != nil {
			return err
		}
		refundTxn.TransactionSignatures[0].Signature = cryptoSig1[:]

		// Recieving entity never communitcates, funding entity must reclaim
		// the 'channelSize' coins that were intended to go to the channel.
		reclaimUC, err := cst.wallet.NextAddress()
		reclaimAddr := reclaimUC.UnlockHash()
		if err != nil {
			return err
		}
		reclaimTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         fundOutputID,
				UnlockConditions: channelFundingUC,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: reclaimAddr,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(fundOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}
		sigHash = reclaimTxn.SigHash(0)
		cryptoSig, err := crypto.SignHash(sigHash, channelFundingSK)
		if err != nil {
			return err
		}
		reclaimTxn.TransactionSignatures[0].Signature = cryptoSig[:]
		err = cst.tpool.AcceptTransactionSet(append(fundTxnSet, reclaimTxn))
		if err != nil {
			return err
		}
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}
		reclaimOutputID := reclaimTxn.SiacoinOutputID(0)
		exists := cst.cs.db.inSiacoinOutputs(reclaimOutputID)
		if !exists {
			return errors.New("failed to reclaim an output that belongs to the funding entity")
		}
	}

	// Create a channel and the open the channel, but close the channel using
	// the timelocked signature.
	{
		// Funding entity creates but does not sign a transaction that funds the
		// channel address. Because the wallet is not very flexible, the channel
		// txn needs to be fully custom. To get a custom txn, manually create an
		// address and then use the wallet to fund that address.
		channelSize := types.NewCurrency64(10e3)
		channelFundingSK, channelFundingPK, err := crypto.GenerateSignatureKeys()
		if err != nil {
			return err
		}
		channelFundingUC := types.UnlockConditions{
			PublicKeys: []types.SiaPublicKey{{
				Algorithm: types.SignatureEd25519,
				Key:       channelFundingPK[:],
			}},
			SignaturesRequired: 1,
		}
		channelFundingAddr := channelFundingUC.UnlockHash()
		fundTxnBuilder := cst.wallet.StartTransaction()
		err = fundTxnBuilder.FundSiacoins(channelSize)
		if err != nil {
			return err
		}
		scoFundIndex := fundTxnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: channelSize, UnlockHash: channelFundingAddr})
		fundTxnSet, err := fundTxnBuilder.Sign(true)
		if err != nil {
			return err
		}
		fundOutputID := fundTxnSet[len(fundTxnSet)-1].SiacoinOutputID(int(scoFundIndex))
		channelTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         fundOutputID,
				UnlockConditions: channelFundingUC,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: channelAddress,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(fundOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}

		// Funding entity creates and signs a transaction that spends the full
		// channel output.
		channelOutputID := channelTxn.SiacoinOutputID(0)
		refundUC, err := cst.wallet.NextAddress()
		refundAddr := refundUC.UnlockHash()
		if err != nil {
			return err
		}
		refundTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         channelOutputID,
				UnlockConditions: uc,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: refundAddr,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}
		sigHash := refundTxn.SigHash(0)
		cryptoSig1, err := crypto.SignHash(sigHash, sk1)
		if err != nil {
			return err
		}
		refundTxn.TransactionSignatures[0].Signature = cryptoSig1[:]

		// Receiving entity signs the transaction that spends the full channel
		// output, but with a timelock.
		refundTxn.TransactionSignatures = append(refundTxn.TransactionSignatures, types.TransactionSignature{
			ParentID:       crypto.Hash(channelOutputID),
			PublicKeyIndex: 1,
			Timelock:       cst.cs.dbBlockHeight() + 2,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		})
		sigHash = refundTxn.SigHash(1)
		cryptoSig2, err := crypto.SignHash(sigHash, sk2)
		if err != nil {
			return err
		}
		refundTxn.TransactionSignatures[1].Signature = cryptoSig2[:]

		// Funding entity will now sign and broadcast the funding transaction.
		sigHash = channelTxn.SigHash(0)
		cryptoSig0, err := crypto.SignHash(sigHash, channelFundingSK)
		if err != nil {
			return err
		}
		channelTxn.TransactionSignatures[0].Signature = cryptoSig0[:]
		err = cst.tpool.AcceptTransactionSet(append(fundTxnSet, channelTxn))
		if err != nil {
			return err
		}
		// Put the txn in a block.
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}

		// Receiving entity never signs another transaction, so the funding
		// entity waits until the timelock is complete, and then submits the
		// refundTxn.
		for i := 0; i < 3; i++ {
			block, _ := cst.miner.FindBlock()
			err = cst.cs.AcceptBlock(block)
			if err != nil {
				return err
			}
		}
		err = cst.tpool.AcceptTransactionSet([]types.Transaction{refundTxn})
		if err != nil {
			return err
		}
		block, _ = cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}
		refundOutputID := refundTxn.SiacoinOutputID(0)
		exists := cst.cs.db.inSiacoinOutputs(refundOutputID)
		if !exists {
			return errors.New("timelocked refund transaction did not get spent correctly")
		}
	}

	return nil
}

// TestPaymentChannelBlocks creates a consensus set tester and uses it to call
// testPaymentChannelBlocks.
func TestPaymentChannelBlocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestPaymentChannelBlocks")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	err = cst.testPaymentChannelBlocks()
	if err != nil {
		t.Fatal(err)
	}
}

// TestBuriedBadFork creates a block with an invalid transaction that's not on
// the longest fork. The consensus set will not validate that block. Then valid
// blocks are added on top of it to make it the longest fork. When it becomes
// the longest fork, all the blocks should be fully validated and thrown out
// because a parent is invalid.
func TestBuriedBadFork(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst, err := createConsensusSetTester("TestBuriedBadFork")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	pb := cst.cs.dbCurrentProcessedBlock()

	// Create a bad block that builds on a parent, so that it is part of not
	// the longest fork.
	badBlock := types.Block{
		ParentID:     pb.Block.ParentID,
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height)}},
		Transactions: []types.Transaction{{
			SiacoinInputs: []types.SiacoinInput{{}}, // Will trigger an error on full verification but not partial verification.
		}},
	}
	parent, err := cst.cs.dbGetBlockMap(pb.Block.ParentID)
	if err != nil {
		t.Fatal(err)
	}
	badBlock, _ = cst.miner.SolveBlock(badBlock, parent.ChildTarget)
	err = cst.cs.AcceptBlock(badBlock)
	if err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}

	// Build another bock on top of the bad block that is fully valid, this
	// will cause a fork and full validation of the bad block, both the bad
	// block and this block should be thrown away.
	block := types.Block{
		ParentID:     badBlock.ID(),
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height + 1)}},
	}
	block, _ = cst.miner.SolveBlock(block, parent.ChildTarget) // okay because the target will not change
	err = cst.cs.AcceptBlock(block)
	if err == nil {
		t.Fatal("a bad block failed to cause an error")
	}
}

// TestBuriedBadTransaction tries submitting a block with a bad transaction
// that is buried under good transactions.
func TestBuriedBadTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBuriedBadTransaction")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	pb := cst.cs.dbCurrentProcessedBlock()

	// Create a good transaction using the wallet.
	txnValue := types.NewCurrency64(1200)
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(txnValue)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: txnValue})
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}

	// Create a bad transaction
	badTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{}},
	}
	txns := append(cst.tpool.TransactionList(), badTxn)

	// Create a block with a buried bad transaction.
	block := types.Block{
		ParentID:     pb.Block.ID(),
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height + 1)}},
		Transactions: txns,
	}
	block, _ = cst.miner.SolveBlock(block, pb.ChildTarget)
	err = cst.cs.AcceptBlock(block)
	if err == nil {
		t.Error("buried transaction didn't cause an error")
	}
}

// COMPATv0.4.0
//
// This test checks that the hardfork scheduled for block 12,000 rolls through
// smoothly.
/*
func TestTaxHardfork(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestTaxHardfork")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a file contract with a payout that is put into the blockchain
	// before the hardfork block but expires after the hardfork block.
	payout := types.NewCurrency64(400e6)
	fc := types.FileContract{
		WindowStart:        cst.cs.height() + 10,
		WindowEnd:          cst.cs.height() + 12,
		Payout:             payout,
		ValidProofOutputs:  []types.SiacoinOutput{{}},
		MissedProofOutputs: []types.SiacoinOutput{{}},
	}
	outputSize := payout.Sub(fc.Tax())
	fc.ValidProofOutputs[0].Value = outputSize
	fc.MissedProofOutputs[0].Value = outputSize

	// Create and fund a transaction with a file contract.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddFileContract(fc)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the siafund pool was increased.
	var siafundPool types.Currency
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		siafundPool = getSiafundPool(tx)
		return nil
	})
	if err != nil {
		panic(err)
	}
	if siafundPool.Cmp(types.NewCurrency64(15590e3)) != 0 {
		t.Fatal("siafund pool was not increased correctly")
	}

	// Mine blocks until the file contract expires and see if any problems
	// occur.
	for i := 0; i < 12; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
}
*/
