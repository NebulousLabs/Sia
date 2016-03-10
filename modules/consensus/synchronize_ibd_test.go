package consensus

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
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
		if err != nil {
			t.Fatal(err)
		}
		defer cst.Close()
		remoteCSTs[i] = cst
	}
	// Create the "local" peer.
	localCST, err := blankConsensusSetTester("TestSimpleInitialBlockchainDownload - local")
	if err != nil {
		t.Fatal(err)
	}
	defer localCST.Close()
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

type mockGatewayRPCError struct {
	modules.Gateway
	rpcErrs map[modules.NetAddress]error
}

func (g *mockGatewayRPCError) RPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	return g.rpcErrs[addr]
}

// TestInitialBlockChainDownloadDisconnects tests that
// threadedInitialBlockchainDownload only disconnects from peers that error
// with anything but a timeout.
func TestInitialBlockchainDownloadDisconnects(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, "TestInitialBlockchainDownloadDisconnects")
	g, err := gateway.New("localhost:0", filepath.Join(testdir, "local", modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	mg := mockGatewayRPCError{
		Gateway: g,
		rpcErrs: make(map[modules.NetAddress]error),
	}
	localCS, err := New(&mg, filepath.Join(testdir, "local", modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	defer localCS.Close()

	rpcErrs := []error{
		// rpcErrs that should cause a a disconnect.
		io.EOF,
		errors.New("random error"),
		errSendBlocksStalled,
		// rpcErrs that should not cause a disconnect.
		mockNetError{
			error:   errors.New("mock timeout error"),
			timeout: true,
		},
		// Need at least minNumOutbound peers that return nil for
		// threadedInitialBlockchainDownload to mark IBD done.
		nil, nil, nil, nil, nil,
	}
	for i, rpcErr := range rpcErrs {
		g, err := gateway.New("localhost:0", filepath.Join(testdir, "remote - "+strconv.Itoa(i), modules.GatewayDir))
		if err != nil {
			t.Fatal(err)
		}
		defer g.Close()
		err = localCS.gateway.Connect(g.Address())
		if err != nil {
			t.Fatal(err)
		}
		mg.rpcErrs[g.Address()] = rpcErr
	}
	// Sleep to to give the OnConnectRPCs time to finish.
	time.Sleep(500 * time.Millisecond)
	// Do IBD.
	localCS.threadedInitialBlockchainDownload()
	// Check that localCS disconnected from peers that errored but did not time out during SendBlocks.
	for _, p := range localCS.gateway.Peers() {
		err = mg.rpcErrs[p.NetAddress]
		if err == nil {
			continue
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			continue
		}
		t.Fatalf("threadedInitialBlockchainDownload didn't disconnect from a peer that returned '%v'", err)
	}
	if len(localCS.gateway.Peers()) != 6 {
		t.Error("threadedInitialBlockchainDownload disconnected from peers that timedout or didn't error")
	}
}
