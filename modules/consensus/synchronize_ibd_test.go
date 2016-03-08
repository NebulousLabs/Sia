package consensus

import (
	"fmt"
	"testing"
	"time"
)

// TestSimpleInitialBlockchainDownload tests that
// threadedInitialBlockchainDownload synchronizes with peers in the simple case
// where there are 8 outbound peers with the same blockchain.
func TestSimpleInitialBlockchainDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create 8 remote peers.
	remoteCSTs := make([]*consensusSetTester, 8)
	for i := range remoteCSTs {
		cst, err := blankConsensusSetTester(fmt.Sprintf("TestSimpleInitialBlockchainDownload - %v", i))
		defer cst.Close()
		if err != nil {
			t.Fatal(err)
		}
		remoteCSTs[i] = cst
	}
	// Create the "local" peer.
	localCST, err := blankConsensusSetTester("TestSimpleInitialBlockchainDownload - local")
	if err != nil {
		t.Fatal(err)
	}
	for _, cst := range remoteCSTs {
		err = localCST.cs.gateway.Connect(cst.cs.gateway.Address())
		if err != nil {
			t.Fatal(err)
		}
	}
	// Give the OnConnectRPCs time to finish.
	time.Sleep(1 * time.Second)

	// Test IBD when all peers have only the genesis block.
	doneChan := make(chan struct{})
	go func() {
		localCST.cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	select {
	case <-doneChan:
	case <-time.After(5 * time.Second):
		t.Fatal("initialBlockchainDownload never completed")
	}
	if localCST.cs.CurrentBlock().ID() != remoteCSTs[0].cs.CurrentBlock().ID() {
		t.Fatalf("current block ids do not match: expected '%v', got '%v'", remoteCSTs[0].cs.CurrentBlock().ID(), localCST.cs.CurrentBlock().ID())
	}

	// Test IBD when all remote peers have the same longest chain.
	for i := 0; i < 20; i++ {
		b, err := remoteCSTs[0].miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		for _, cst := range remoteCSTs {
			err = cst.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	go func() {
		localCST.cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	select {
	case <-doneChan:
	case <-time.After(5 * time.Second):
		t.Fatal("initialBlockchainDownload never completed")
	}
	if localCST.cs.CurrentBlock().ID() != remoteCSTs[0].cs.CurrentBlock().ID() {
		t.Fatalf("current block ids do not match: expected '%v', got '%v'", remoteCSTs[0].cs.CurrentBlock().ID(), localCST.cs.CurrentBlock().ID())
	}

	// Test IBD when not starting from the genesis block.
	for i := 0; i < 4; i++ {
		b, err := remoteCSTs[0].miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		for _, cst := range remoteCSTs {
			err = cst.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	go func() {
		localCST.cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	select {
	case <-doneChan:
	case <-time.After(5 * time.Second):
		t.Fatal("initialBlockchainDownload never completed")
	}
	if localCST.cs.CurrentBlock().ID() != remoteCSTs[0].cs.CurrentBlock().ID() {
		t.Fatalf("current block ids do not match: expected '%v', got '%v'", remoteCSTs[0].cs.CurrentBlock().ID(), localCST.cs.CurrentBlock().ID())
	}

	// Test IBD when the remote peers are on a longer fork.
	for i := 0; i < 5; i++ {
		b, err := localCST.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = localCST.cs.managedAcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		b, err := remoteCSTs[0].miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		for _, cst := range remoteCSTs {
			err = cst.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	go func() {
		localCST.cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	select {
	case <-doneChan:
	case <-time.After(5 * time.Second):
		t.Fatal("initialBlockchainDownload never completed")
	}
	if localCST.cs.CurrentBlock().ID() != remoteCSTs[0].cs.CurrentBlock().ID() {
		t.Fatalf("current block ids do not match: expected '%v', got '%v'", remoteCSTs[0].cs.CurrentBlock().ID(), localCST.cs.CurrentBlock().ID())
	}

	// Test IBD when the remote peers are on a shorter fork.
	for i := 0; i < 10; i++ {
		b, err := localCST.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = localCST.cs.managedAcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		b, err := remoteCSTs[0].miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		for _, cst := range remoteCSTs {
			err = cst.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	localCurrentBlock := localCST.cs.CurrentBlock()
	go func() {
		localCST.cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	select {
	case <-doneChan:
	case <-time.After(5 * time.Second):
		t.Fatal("initialBlockchainDownload never completed")
	}
	if localCST.cs.CurrentBlock().ID() != localCurrentBlock.ID() {
		t.Fatalf("local was on a longer fork and should not have reorged")
	}
	if localCST.cs.CurrentBlock().ID() == remoteCSTs[0].cs.CurrentBlock().ID() {
		t.Fatalf("ibd syncing is one way, and a longer fork on the local cs should not cause a reorg on the remote cs's")
	}
}
