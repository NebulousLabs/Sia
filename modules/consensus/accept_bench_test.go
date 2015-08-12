package consensus

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/types"
)

// benchmarkEmptyBlocks is a benchmark that mines many blocks, and
// measures how long it takes to add them to the consensusset
func benchmarkAcceptEmptyBlocks(b *testing.B) error {
	// Create an alternate testing consensus set, which does not
	// have any subscribers
	testdir := build.TempDir(modules.ConsensusDir, "BenchmarkEmptyBlocksB")
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return err
	}
	cs, err := New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return errors.New("Error creating consensus: " + err.Error())
	}
	defer cs.Close()

	// The test dir will be reset each time the benchmark
	// is done.
	cst, err := createConsensusSetTester("BenchmarkEmptyBlocks")
	if err != nil {
		return errors.New("Error creating tester: " + err.Error())
	}
	defer cst.closeCst()
	h := cst.cs.db.pathHeight()
	for i := types.BlockHeight(1); i < h; i++ {
		err = cs.AcceptBlock(cst.cs.db.getBlockMap(cst.cs.db.getPath(i)).Block)
		if err != nil {
			return err
		}
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		// Submit a block to the consensus set tester - which has many
		// subscribers. (untimed)
		b.StopTimer()
		block, err := cst.miner.FindBlock()
		if err != nil {
			return err
		}
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			errstr := fmt.Sprintf("Error accepting %d from mined: %s", j, err.Error())
			return errors.New(errstr)
		}
		b.StartTimer()

		// Submit a block to the consensus set which has no subscribers.
		// (timed)
		err = cs.AcceptBlock(block)
		if err != nil {
			errstr := fmt.Sprintf("Error accepting %d for timing: %s", j, err.Error())
			return errors.New(errstr)
		}
	}

	return nil
}

// BenchmarkEmptyBlocks is a wrapper for benchmarkEmptyBlocks, which
// handles error catching
func BenchmarkAcceptEmptyBlocks(b *testing.B) {
	b.ReportAllocs()
	err := benchmarkAcceptEmptyBlocks(b)
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkAcceptBigTxBlocks(b *testing.B) {
	b.ReportAllocs()

	numSigs := 7

	cst, err := createConsensusSetTester("BenchmarkEmptyBlocksA")
	if err != nil {
		b.Fatal(err)
	}
	defer cst.closeCst()

	// Mine until the wallet has 100 utxos
	for cst.cs.height() < (types.BlockHeight(numSigs) + types.MaturityDelay) {
		_, err := cst.miner.AddBlock()
		if err != nil {
			b.Fatal(err)
		}
	}

	// Create an alternate testing consensus set, which does not
	// have any subscribers
	testdir := build.TempDir(modules.ConsensusDir, "BenchmarkEmptyBlocksB")
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		b.Fatal(err)
	}
	cs, err := New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		b.Fatal("Error creating consensus: " + err.Error())
	}
	defer cs.Close()
	h := cst.cs.db.pathHeight()
	for i := types.BlockHeight(1); i < h; i++ {
		err = cs.AcceptBlock(cst.cs.db.getBlockMap(cst.cs.db.getPath(i)).Block)
		if err != nil {
			b.Fatal(err)
		}
	}

	// construct a transaction using numSigs utxo's, and signed numSigs times
	outputValues := make([]types.Currency, numSigs)
	txValue := types.ZeroCurrency
	for i := 1; i <= numSigs; i++ {
		outputValues[i-1] = types.CalculateCoinbase(types.BlockHeight(i))
		txValue = txValue.Add(outputValues[i-1])
	}

	b.ResetTimer()
	b.StopTimer()
	for j := 0; j < b.N; j++ {
		txnBuilder := cst.wallet.StartTransaction()
		err = txnBuilder.FundSiacoins(txValue)
		if err != nil {
			b.Fatal(err)
		}

		for i := 0; i < numSigs; i++ {
			unlockConditions, err := cst.wallet.NextAddress()
			if err != nil {
				b.Fatal(err)
			}
			txnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: outputValues[i], UnlockHash: unlockConditions.UnlockHash()})
		}

		txnSet, err := txnBuilder.Sign(true)
		if err != nil {
			b.Fatal(err)
		}

		outputVolume := types.ZeroCurrency
		for _, out := range txnSet[0].SiacoinOutputs {
			outputVolume = outputVolume.Add(out.Value)
		}

		blk := types.Block{
			ParentID:  cst.cs.CurrentBlock().ID(),
			Timestamp: types.CurrentTimestamp(),
			MinerPayouts: []types.SiacoinOutput{
				{Value: types.CalculateCoinbase(cst.cs.height())},
			},
			Transactions: txnSet,
		}

		target, _ := cst.cs.ChildTarget(cst.cs.CurrentBlock().ID())
		block, _ := cst.miner.SolveBlock(blk, target)
		// Submit it to the first consensus set for validity
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		// Time the consensus set without subscribers
		err = cs.AcceptBlock(block)
		if err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
	}
}
