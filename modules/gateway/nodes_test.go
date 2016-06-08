package gateway

import (
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const dummyNode = "111.111.111.111:1111"

func TestAddNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestAddNode", t)
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

func TestRemoveNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestRemoveNode", t)
	defer g.Close()
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

func TestRandomNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestRandomNode", t)
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

func TestShareNodes(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g1 := newTestingGateway("TestShareNodes1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestShareNodes2", t)
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
	g1.nodes = map[modules.NetAddress]struct{}{}
	g1.mu.Unlock()
	g2.mu.Lock()
	g2.nodes = map[modules.NetAddress]struct{}{}
	g2.mu.Unlock()

	// SharePeers should now return no peers
	var nodes []modules.NetAddress
	err = g1.RPC(g2.Address(), "ShareNodes", func(conn modules.PeerConn) error {
		return encoding.ReadObject(conn, &nodes, maxSharedNodes*maxAddrLength)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatal("gateway gave non-existent addresses:", nodes)
	}

	// sharing should be capped at maxSharedNodes
	for i := 1; i < maxSharedNodes+11; i++ {
		g2.mu.Lock()
		err := g2.addNode(modules.NetAddress("111.111.111.111:" + strconv.Itoa(i)))
		g2.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = g1.RPC(g2.Address(), "ShareNodes", func(conn modules.PeerConn) error {
		return encoding.ReadObject(conn, &nodes, maxSharedNodes*maxAddrLength)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != maxSharedNodes {
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
	g1 := newTestingGateway("TestNodesAreSharedOnConnect1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestNodesAreSharedOnConnect2", t)
	defer g2.Close()
	g3 := newTestingGateway("TestNodesAreSharedOnConnect3", t)
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
