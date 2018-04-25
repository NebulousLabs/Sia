package gateway

import (
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/fastrand"
)

const dummyNode = "111.111.111.111:1111"

// TestAddNode tries adding a node to the gateway using the unexported addNode
// function. Edge case trials are also performed.
func TestAddNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()

	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.addNode(dummyNode); err != nil {
		t.Fatal("addNode failed:", err)
	}
	if err := g.addNode(dummyNode); err != errNodeExists {
		t.Error("addNode added duplicate node")
	}
	if err := g.addNode("foo"); err == nil {
		t.Error("addNode added unroutable address")
	}
	if err := g.addNode("foo:9981"); err == nil {
		t.Error("addNode added a non-IP address")
	}
	if err := g.addNode("[::]:9981"); err == nil {
		t.Error("addNode added unspecified address")
	}
	if err := g.addNode(g.myAddr); err != errOurAddress {
		t.Error("addNode added our own address")
	}
}

// TestRemoveNode tries remiving a node from the gateway.
func TestRemoveNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g := newTestingGateway(t)
	defer g.Close()
	t.Parallel()

	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.addNode(dummyNode); err != nil {
		t.Fatal("addNode failed:", err)
	}
	if err := g.removeNode(dummyNode); err != nil {
		t.Fatal("removeNode failed:", err)
	}
	if err := g.removeNode("bar"); err == nil {
		t.Fatal("removeNode removed nonexistent node")
	}
}

// TestRandomNode tries pulling random nodes from the gateway using
// g.randomNode() under a variety of conditions.
func TestRandomNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()

	// Test with 0 nodes.
	g.mu.RLock()
	_, err := g.randomNode()
	g.mu.RUnlock()
	if err != errNoPeers {
		t.Fatal("randomNode should fail when the gateway has 0 nodes")
	}

	// Test with 1 node.
	g.mu.Lock()
	if err = g.addNode(dummyNode); err != nil {
		t.Fatal(err)
	}
	g.mu.Unlock()
	g.mu.RLock()
	addr, err := g.randomNode()
	g.mu.RUnlock()
	if err != nil {
		t.Fatal("randomNode failed:", err)
	} else if addr != dummyNode {
		t.Fatal("randomNode returned wrong address:", addr)
	}

	// Test again with 0 nodes.
	g.mu.Lock()
	err = g.removeNode(dummyNode)
	g.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	g.mu.RLock()
	_, err = g.randomNode()
	g.mu.RUnlock()
	if err != errNoPeers {
		t.Fatalf("randomNode returned wrong error: expected %v, got %v", errNoPeers, err)
	}

	// Test with 3 nodes.
	nodes := map[modules.NetAddress]int{
		"111.111.111.111:1111": 0,
		"111.111.111.111:2222": 0,
		"111.111.111.111:3333": 0,
	}
	g.mu.Lock()
	for addr := range nodes {
		err := g.addNode(addr)
		if err != nil {
			t.Error(err)
		}
	}
	g.mu.Unlock()

	for i := 0; i < len(nodes)*10; i++ {
		g.mu.RLock()
		addr, err := g.randomNode()
		g.mu.RUnlock()
		if err != nil {
			t.Fatal("randomNode failed:", err)
		}
		nodes[addr]++
	}
	for node, count := range nodes {
		if count == 0 { // 1-in-200000 chance of occurring naturally
			t.Errorf("node %v was never selected", node)
		}
	}
}

// TestShareNodes checks that two gateways will share nodes with eachother
// following the desired sharing strategy.
func TestShareNodes(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g1 := newNamedTestingGateway(t, "1")
	defer g1.Close()
	g2 := newNamedTestingGateway(t, "2")
	defer g2.Close()

	// add a node to g2
	g2.mu.Lock()
	err := g2.addNode(dummyNode)
	g2.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	// connect
	err = g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("couldn't connect:", err)
	}

	err = build.Retry(50, 100*time.Millisecond, func() error {
		g1.mu.Lock()
		_, exists := g1.nodes[dummyNode]
		g1.mu.Unlock()
		if !exists {
			return errors.New("node not added")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// g1 should have received the node
	time.Sleep(100 * time.Millisecond)
	g1.mu.Lock()
	err = g1.addNode(dummyNode)
	g1.mu.Unlock()
	if err == nil {
		t.Fatal("gateway did not receive nodes during Connect:", g1.nodes)
	}

	// remove all nodes from both peers
	g1.mu.Lock()
	g1.nodes = map[modules.NetAddress]*node{}
	g1.mu.Unlock()
	g2.mu.Lock()
	g2.nodes = map[modules.NetAddress]*node{}
	g2.mu.Unlock()

	// SharePeers should now return no peers
	var nodes []modules.NetAddress
	err = g1.RPC(g2.Address(), "ShareNodes", func(conn modules.PeerConn) error {
		return encoding.ReadObject(conn, &nodes, maxSharedNodes*modules.MaxEncodedNetAddressLength)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatal("gateway gave non-existent addresses:", nodes)
	}

	// sharing should be capped at maxSharedNodes
	for i := 1; i < int(maxSharedNodes)+11; i++ {
		g2.mu.Lock()
		err := g2.addNode(modules.NetAddress("111.111.111.111:" + strconv.Itoa(i)))
		g2.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = g1.RPC(g2.Address(), "ShareNodes", func(conn modules.PeerConn) error {
		return encoding.ReadObject(conn, &nodes, maxSharedNodes*modules.MaxEncodedNetAddressLength)
	})
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(nodes)) != maxSharedNodes {
		t.Fatalf("gateway gave wrong number of nodes: expected %v, got %v", maxSharedNodes, len(nodes))
	}
}

// TestNodesAreSharedOnConnect tests that nodes that a gateway has never seen
// before are added to the node list when connecting to another gateway that
// has seen said nodes.
func TestNodesAreSharedOnConnect(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g1 := newNamedTestingGateway(t, "1")
	defer g1.Close()
	g2 := newNamedTestingGateway(t, "2")
	defer g2.Close()
	g3 := newNamedTestingGateway(t, "3")
	defer g3.Close()

	// connect g2 to g1
	err := g2.Connect(g1.Address())
	if err != nil {
		t.Fatal("couldn't connect:", err)
	}

	// connect g3 to g1
	err = g3.Connect(g1.Address())
	if err != nil {
		t.Fatal("couldn't connect:", err)
	}

	// g3 should have received g2's address from g1
	time.Sleep(200 * time.Millisecond)
	g3.mu.Lock()
	defer g3.mu.Unlock()
	if _, ok := g3.nodes[g2.Address()]; !ok {
		t.Fatal("node was not relayed:", g3.nodes)
	}
}

// TestPruneNodeThreshold checks that gateways will not purge nodes if they are
// below the purge threshold, even if those nodes are offline.
func TestPruneNodeThreshold(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// The next part of the test expects the pruneNodeListLen to be at least
	// 'maxSharedNodes * 2 + 2 in size.
	if uint64(pruneNodeListLen) < (maxSharedNodes*2)+2 {
		t.Fatal("Constants do not match test, please either adjust the constants or refactor this test", maxSharedNodes, pruneNodeListLen)
	}

	// Create and connect pruneNodeListLen gateways.
	var gs []*Gateway
	for i := 0; i < pruneNodeListLen; i++ {
		gs = append(gs, newNamedTestingGateway(t, strconv.Itoa(i)))

		// Connect this gateway to the previous gateway.
		if i != 0 {
			err := gs[i].Connect(gs[i-1].myAddr)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Spin until all gateways have a nearly full node list.
	success := false
	for i := 0; i < 50; i++ {
		success = true
		for _, g := range gs {
			g.mu.RLock()
			gNodeLen := len(g.nodes)
			g.mu.RUnlock()
			if gNodeLen < pruneNodeListLen-2 {
				success = false
				break
			}
		}
		if !success {
			time.Sleep(time.Second * 1)
		}
	}
	if !success {
		t.Fatal("peers are not sharing nodes with eachother")
	}

	// Gateway node lists have been filled out. Take a bunch of gateways
	// offline and verify that they do not start pruning eachother.
	var wg sync.WaitGroup
	for i := 2; i < len(gs); i++ {
		wg.Add(1)
		go func(i int) {
			err := gs[i].Close()
			if err != nil {
				panic(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Wait for 5 iterations of the node purge loop. Then verify that the
	// remaining gateways have not been purging nodes.
	time.Sleep(nodePurgeDelay * 5)

	// Check that the remaining gateways have not purged any nodes.
	gs[0].mu.RLock()
	gs0Nodes := len(gs[0].nodes)
	gs[0].mu.RUnlock()
	gs[1].mu.RLock()
	gs1Nodes := len(gs[1].nodes)
	gs[1].mu.RUnlock()
	if gs0Nodes < pruneNodeListLen-2 {
		t.Error("gateway seems to be pruning nodes below purge threshold")
	}
	if gs1Nodes < pruneNodeListLen-2 {
		t.Error("gateway seems to be pruning nodes below purge threshold")
	}

	// Close the remaining gateways.
	err := gs[0].Close()
	if err != nil {
		t.Error(err)
	}
	err = gs[1].Close()
	if err != nil {
		t.Error(err)
	}
}

// TestHealthyNodeListPruning checks that gateways will purge nodes if they are at
// a healthy node threshold and the nodes are offline.
func TestHealthyNodeListPruning(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create and connect healthyNodeListLen*2 gateways.
	var gs []*Gateway
	for i := 0; i < healthyNodeListLen*2; i++ {
		gs = append(gs, newNamedTestingGateway(t, strconv.Itoa(i)))

		// Connect this gateway to the previous gateway.
		if i != 0 {
			err := gs[i].Connect(gs[i-1].myAddr)
			if err != nil {
				t.Fatal(err)
			}
		}
		// To help speed the test up, also connect this gateway to the peer two
		// back.
		if i > 1 {
			err := gs[i].Connect(gs[i-2].myAddr)
			if err != nil {
				t.Fatal(err)
			}
		}
		// To help speed the test up, also connect this gateway to a random
		// previous peer.
		if i > 2 {
			err := gs[i].Connect(gs[fastrand.Intn(i-2)].myAddr)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Spin until all gateways have a nearly full node list.
	err := build.Retry(1000, 100*time.Millisecond, func() error {
		for _, g := range gs {
			g.mu.RLock()
			gNodeLen := len(g.nodes)
			g.mu.RUnlock()
			if gNodeLen < healthyNodeListLen {
				return errors.New("node is not connected to a sufficient number of peers")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal("peers are not sharing nodes with eachother")
	}

	// Gateway node lists have been filled out. Take a bunch of gateways
	// offline and verify that the remaining gateways begin pruning their
	// nodelist.
	var wg sync.WaitGroup
	for i := 2; i < len(gs); i++ {
		wg.Add(1)
		go func(i int) {
			err := gs[i].Close()
			if err != nil {
				panic(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Wait for enough iterations of the node purge loop that over-pruning is
	// possible. (Over-pruning does not need to be guaranteed, causing this
	// test to fail once in a while is sufficient.)
	time.Sleep(nodePurgeDelay * time.Duration(healthyNodeListLen-pruneNodeListLen) * 12)

	// Check that the remaining gateways have pruned nodes.
	gs[0].mu.RLock()
	gs0Nodes := len(gs[0].nodes)
	gs[0].mu.RUnlock()
	gs[1].mu.RLock()
	gs1Nodes := len(gs[1].nodes)
	gs[1].mu.RUnlock()
	if gs0Nodes >= healthyNodeListLen-1 {
		t.Error("gateway is not pruning nodes", healthyNodeListLen, gs0Nodes)
	}
	if gs1Nodes >= healthyNodeListLen-1 {
		t.Error("gateway is not pruning nodes", healthyNodeListLen, gs1Nodes)
	}
	if gs0Nodes < pruneNodeListLen {
		t.Error("gateway is pruning too many nodes", gs0Nodes, pruneNodeListLen)
	}
	if gs1Nodes < pruneNodeListLen {
		t.Error("gateway is pruning too many nodes", gs1Nodes, pruneNodeListLen)
	}

	// Close the remaining gateways.
	err = gs[0].Close()
	if err != nil {
		t.Error(err)
	}
	err = gs[1].Close()
	if err != nil {
		t.Error(err)
	}
}
