package consensus

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/types"
)

// TestSimpleInitialBlockchainDownload tests that
// threadedInitialBlockchainDownload synchronizes with peers in the simple case
// where there are 8 outbound peers with the same blockchain.
func TestSimpleInitialBlockchainDownload(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}

	// Create 8 remote peers.
	remoteCSTs := make([]*consensusSetTester, 8)
	for i := range remoteCSTs {
		cst, err := blankConsensusSetTester(t.Name()+strconv.Itoa(i), modules.ProdDependencies)
		if err != nil {
			t.Fatal(err)
		}
		defer cst.Close()
		remoteCSTs[i] = cst
	}
	// Create the "local" peer.
	localCST, err := blankConsensusSetTester(t.Name()+"- local", modules.ProdDependencies)
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
	time.Sleep(5 * time.Second)

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
			_, err = cst.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil && err != modules.ErrBlockKnown && err != modules.ErrNonExtendingBlock {
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
			_, err = cst.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil && err != modules.ErrBlockKnown {
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
		_, err = localCST.cs.managedAcceptBlocks([]types.Block{b})
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
			_, err = cst.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil && err != modules.ErrBlockKnown {
				t.Log(i)
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
		_, err = localCST.cs.managedAcceptBlocks([]types.Block{b})
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
			_, err = cst.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil && err != modules.ErrBlockKnown {
				t.Log(i)
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
	mu      sync.Mutex
}

func (g *mockGatewayRPCError) RPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rpcErrs[addr]
}

// TestInitialBlockChainDownloadDisconnects tests that
// threadedInitialBlockchainDownload only disconnects from peers that error
// with anything but a timeout.
func TestInitialBlockchainDownloadDisconnects(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, "local", modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	mg := mockGatewayRPCError{
		Gateway: g,
		rpcErrs: make(map[modules.NetAddress]error),
	}
	localCS, err := New(&mg, false, filepath.Join(testdir, "local", modules.ConsensusDir))
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
			error:   errors.New("Read timeout"),
			timeout: true,
		},
		// Need at least minNumOutbound peers that return nil for
		// threadedInitialBlockchainDownload to mark IBD done.
		nil, nil, nil, nil, nil,
	}
	for i, rpcErr := range rpcErrs {
		g, err := gateway.New("localhost:0", false, filepath.Join(testdir, "remote - "+strconv.Itoa(i), modules.GatewayDir))
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
	if len(localCS.gateway.Peers()) != 6 {
		t.Error("threadedInitialBlockchainDownload disconnected from peers that timedout or didn't error", len(localCS.gateway.Peers()))
	}
	for _, p := range localCS.gateway.Peers() {
		err = mg.rpcErrs[p.NetAddress]
		if err == nil {
			continue
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			continue
		}
		t.Fatalf("threadedInitialBlockchainDownload didn't disconnect from a peer that returned '%v', %v", err, p.NetAddress)
	}
}

// TestInitialBlockchainDownloadDoneRules tests that
// threadedInitialBlockchainDownload only terminates under the appropriate
// conditions. Appropriate conditions are:
//  - at least minNumOutbound synced outbound peers
//  - or at least 1 synced outbound peer and minIBDWaitTime has passed since beginning IBD.
func TestInitialBlockchainDownloadDoneRules(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	testdir := build.TempDir(modules.ConsensusDir, t.Name())

	// Create a gateway that can be forced to return errors when its RPC method
	// is called, then create a consensus set using that gateway.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, "local", modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	mg := mockGatewayRPCError{
		Gateway: g,
		rpcErrs: make(map[modules.NetAddress]error),
	}
	cs, err := New(&mg, false, filepath.Join(testdir, "local", modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// Verify that the consensus set will not signal IBD completion when it has
	// zero peers.
	doneChan := make(chan struct{})
	go func() {
		cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	select {
	case <-doneChan:
		t.Error("threadedInitialBlockchainDownload finished with 0 synced peers")
	case <-time.After(minIBDWaitTime + ibdLoopDelay):
	}

	// threadedInitialBlockchainDownload is already running. Feed some inbound
	// peers to the consensus set. The gateway, through its own process of
	// trying to find outbound peers, will eventually convert one of the
	// inbound peers to an outbound peer. IBD should not complete until there
	// is at least one outbound peer.
	//
	// After this function has completed, all of the peers will be shutdown,
	// leaving the consensus set once again with zero peers.
	func() {
		inboundCSTs := make([]*consensusSetTester, 8)
		for i := 0; i < len(inboundCSTs); i++ {
			inboundCST, err := blankConsensusSetTester(filepath.Join(t.Name(), " - inbound "+strconv.Itoa(i)), modules.ProdDependencies)
			if err != nil {
				t.Fatal(err)
			}
			defer inboundCST.Close()
			inboundCST.cs.gateway.Connect(cs.gateway.Address())
		}
		<-doneChan
		peers := cs.gateway.Peers()
		outbound := false
		for _, p := range peers {
			if !p.Inbound {
				outbound = true
				break
			}
		}
		if !outbound {
			t.Error("threadedInitialBlockchainDownload finished with only inbound peers")
		}
	}()

	// Try another initial blockchain download, this time with an outbound peer
	// who is not synced. The consensus set should not determine itself to have
	// completed IBD with only unsynced peers.
	//
	// 'NotSynced' is simulated in this peer by having all RPCs return errors.
	go func() {
		cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	gatewayTimesout, err := gateway.New("localhost:0", false, filepath.Join(testdir, "remote - timesout", modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	defer gatewayTimesout.Close()
	mg.mu.Lock()
	mg.rpcErrs[gatewayTimesout.Address()] = mockNetError{
		error:   errors.New("Read timeout"),
		timeout: true,
	}
	mg.mu.Unlock()
	err = cs.gateway.Connect(gatewayTimesout.Address())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-doneChan:
		t.Error("threadedInitialBlockchainDownload finished with 0 synced peers")
	case <-time.After(minIBDWaitTime + ibdLoopDelay):
	}

	// Add a peer that is synced to the peer that is not synced. IBD should not
	// be considered completed when there is a tie between synced and
	// not-synced peers.
	gatewayNoTimeout, err := gateway.New("localhost:0", false, filepath.Join(testdir, "remote - no timeout1", modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	defer gatewayNoTimeout.Close()
	mg.mu.Lock()
	mg.rpcErrs[gatewayNoTimeout.Address()] = nil
	mg.mu.Unlock()
	err = cs.gateway.Connect(gatewayNoTimeout.Address())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-doneChan:
		t.Fatal("threadedInitialBlockchainDownload finished with 1 synced peer and 1 non-synced peer")
	case <-time.After(minIBDWaitTime + ibdLoopDelay):
	}

	// Test when there is 2 peers that are synced and one that is not synced.
	// There is now a majority synced peers and the minIBDWaitTime has passed,
	// so the IBD function should finish.
	gatewayNoTimeout2, err := gateway.New("localhost:0", false, filepath.Join(testdir, "remote - no timeout2", modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	defer gatewayNoTimeout2.Close()
	mg.mu.Lock()
	mg.rpcErrs[gatewayNoTimeout2.Address()] = nil
	mg.mu.Unlock()
	err = cs.gateway.Connect(gatewayNoTimeout2.Address())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-doneChan:
	case <-time.After(4 * (minIBDWaitTime + ibdLoopDelay)):
		t.Fatal("threadedInitialBlockchainDownload never finished with 2 synced peers and 1 non-synced peer")
	}

	// Test when there are >= minNumOutbound peers and >= minNumOutbound peers are synced.
	gatewayNoTimeouts := make([]modules.Gateway, minNumOutbound-1)
	for i := 0; i < len(gatewayNoTimeouts); i++ {
		tmpG, err := gateway.New("localhost:0", false, filepath.Join(testdir, fmt.Sprintf("remote - no timeout-auto-%v", i+3), modules.GatewayDir))
		if err != nil {
			t.Fatal(err)
		}
		defer tmpG.Close()
		mg.mu.Lock()
		mg.rpcErrs[tmpG.Address()] = nil
		mg.mu.Unlock()
		gatewayNoTimeouts[i] = tmpG
		err = cs.gateway.Connect(gatewayNoTimeouts[i].Address())
		if err != nil {
			t.Fatal(err)
		}
	}
	go func() {
		cs.threadedInitialBlockchainDownload()
		doneChan <- struct{}{}
	}()
	select {
	case <-doneChan:
	case <-time.After(minIBDWaitTime):
		t.Fatal("threadedInitialBlockchainDownload didn't finish in less than minIBDWaitTime")
	}
}

// TestGenesisBlockSync is a regression test that checks what happens when two
// consensus sets with only the genesis block are connected. They should
// determine that they are sync'd, however previously they would not sync to
// eachother as they would report EOF instead of performing correct block
// exchange.
func TestGenesisBlockSync(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}

	// Create two consensus sets that have zero blocks each (except for the
	// genesis block).
	cst1, err := blankConsensusSetTester(t.Name()+"1", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	cst2, err := blankConsensusSetTester(t.Name()+"2", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}

	// Connect them.
	err = cst1.gateway.Connect(cst2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	// Block until both report that they are sync'd.
	for i := 0; i < 100; i++ {
		time.Sleep(time.Millisecond * 100)
		if cst1.cs.Synced() && cst2.cs.Synced() {
			break
		}
	}
	if !cst1.cs.Synced() || !cst2.cs.Synced() {
		t.Error("Consensus sets did not synchronize to eachother", cst1.cs.Synced(), cst2.cs.Synced())
	}

	time.Sleep(time.Second * 12)
	if len(cst1.gateway.Peers()) == 0 {
		t.Error("disconnection occurred!")
	}
}
