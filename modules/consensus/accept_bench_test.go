package consensus

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/types"
)

// BenchmarkAcceptEmptyBlocks measures how quckly empty blocks are integrated
// into the consensus set.
//
// i7-4770, 1d60d69: 1.356 ms / op
func BenchmarkAcceptEmptyBlocks(b *testing.B) {
	cst, err := createConsensusSetTester(b.Name() + strconv.Itoa(b.N))
	if err != nil {
		b.Fatal("Error creating tester: " + err.Error())
	}
	defer cst.Close()

	// Create an alternate testing consensus set, which does not
	// have any subscribers
	testdir := build.TempDir(modules.ConsensusDir, "BenchmarkEmptyBlocks - 2")
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		b.Fatal(err)
	}
	cs, err := New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		b.Fatal(err)
	}
	defer cs.Close()

	// Synchronisze the cst and the subscriberless consensus set.
	h := cst.cs.dbBlockHeight()
	for i := types.BlockHeight(1); i <= h; i++ {
		id, err := cst.cs.dbGetPath(i)
		if err != nil {
			b.Fatal(err)
		}
		processedBlock, err := cst.cs.dbGetBlockMap(id)
		if err != nil {
			b.Fatal(err)
		}
		err = cs.AcceptBlock(processedBlock.Block)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.StopTimer()
	for j := 0; j < b.N; j++ {
		// Submit a block to the consensus set tester - which has many
		// subscribers. (untimed)
		block, err := cst.miner.AddBlock()
		if err != nil {
			b.Fatal(err)
		}

		// Submit a block to the consensus set which has no subscribers.
		// (timed)
		b.StartTimer()
		err = cs.AcceptBlock(block)
		if err != nil {
			b.Fatal("error accepting a block:", err)
		}
		b.StopTimer()
	}
}

// BenchmarkAcceptSmallBlocks measures how quickly smaller blocks are
// integrated into the consensus set.
//
// i7-4770, 1d60d69: 3.579 ms / op
func BenchmarkAcceptSmallBlocks(b *testing.B) {
	cst, err := createConsensusSetTester(b.Name() + strconv.Itoa(b.N))
	if err != nil {
		b.Fatal(err)
	}
	defer cst.Close()

	// COMPAT v0.4.0
	//
	// Push the height of the consensus set tester beyond the fork height.
	for i := 0; i < 10; i++ {
		_, err := cst.miner.AddBlock()
		if err != nil {
			b.Fatal(err)
		}
	}

	// Create an alternate testing consensus set, which does not
	// have any subscribers
	testdir := build.TempDir(modules.ConsensusDir, "BenchmarkAcceptSmallBlocks - 2")
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		b.Fatal(err)
	}
	cs, err := New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		b.Fatal("Error creating consensus: " + err.Error())
	}
	defer cs.Close()

	// Synchronize the consensus set with the consensus set tester.
	h := cst.cs.dbBlockHeight()
	for i := types.BlockHeight(1); i <= h; i++ {
		id, err := cst.cs.dbGetPath(i)
		if err != nil {
			b.Fatal(err)
		}
		processedBlock, err := cst.cs.dbGetBlockMap(id)
		if err != nil {
			b.Fatal(err)
		}
		err = cs.AcceptBlock(processedBlock.Block)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.StopTimer()
	for j := 0; j < b.N; j++ {
		// Create a transaction with a miner fee, a normal siacoin output, and
		// a funded file contract.
		txnBuilder, err := cst.wallet.StartTransaction()
		if err != nil {
			b.Fatal(err)
		}
		err = txnBuilder.FundSiacoins(types.NewCurrency64(125e6))
		if err != nil {
			b.Fatal(err)
		}
		// Add a small miner fee.
		txnBuilder.AddMinerFee(types.NewCurrency64(5e6))
		// Add a siacoin output.
		txnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: types.NewCurrency64(20e6)})
		// Add a file contract.
		fc := types.FileContract{
			WindowStart: 1000,
			WindowEnd:   10005,
			Payout:      types.NewCurrency64(100e6),
			ValidProofOutputs: []types.SiacoinOutput{{
				Value: types.NewCurrency64(96100e3),
			}},
			MissedProofOutputs: []types.SiacoinOutput{{
				Value: types.NewCurrency64(96100e3),
			}},
		}
		txnBuilder.AddFileContract(fc)
		txnSet, err := txnBuilder.Sign(true)
		if err != nil {
			b.Fatal(err)
		}

		// Submit the transaction set and mine the block.
		err = cst.tpool.AcceptTransactionSet(txnSet)
		if err != nil {
			b.Fatal(err)
		}
		block, err := cst.miner.AddBlock()
		if err != nil {
			b.Fatal(err)
		}

		// Submit the block to the consensus set without subscribers, timing
		// how long it takes for the block to get accepted.
		b.StartTimer()
		err = cs.AcceptBlock(block)
		if err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
	}
}
