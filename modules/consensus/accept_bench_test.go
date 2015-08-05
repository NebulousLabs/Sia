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
func benchmarkEmptyBlocks(b *testing.B) error {
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

	// The test dir will be reset each time the benchmark
	// is done.
	cst, err := createConsensusSetTester("BenchmarkEmptyBlocks")
	if err != nil {
		return errors.New("Error creating tester: " + err.Error())
	}
	h := cst.cs.db.pathHeight()
	for i := types.BlockHeight(1); i < h; i++ {
		err = cs.AcceptBlock(cst.cs.db.getBlockMap(cst.cs.db.getPath(i)).Block)
		if err != nil {
			return err
		}
	}

	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		b.StopTimer()
		block, _ := cst.miner.FindBlock()

		err = cst.cs.AcceptBlock(block)
		if err != nil {
			errstr := fmt.Sprintf("Error accepting %d from mined: %s", j, err.Error())
			return errors.New(errstr)
		}
		b.StartTimer()
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
func BenchmarkEmptyBlocks(b *testing.B) {
	b.ReportAllocs()
	err := benchmarkEmptyBlocks(b)
	if err != nil {
		b.Fatal(err)
	}
}
