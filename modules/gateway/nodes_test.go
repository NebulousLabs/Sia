package gateway

import (
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

func TestAddNode(t *testing.T) {
	g := newTestingGateway("TestAddNode", t)
	defer g.Close()
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	if err := g.addNode("foo"); err != nil {
		t.Fatal("addNode failed:", err)
	}
	if err := g.addNode("foo"); err == nil {
		t.Fatal("addNode added duplicate node")
	}
}

func TestRemoveNode(t *testing.T) {
	g := newTestingGateway("TestRemoveNode", t)
	defer g.Close()
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	if err := g.addNode("foo"); err != nil {
		t.Fatal("addNode failed:", err)
	}
	if err := g.removeNode("foo"); err != nil {
		t.Fatal("removeNode failed:", err)
	}
	if err := g.removeNode("bar"); err == nil {
		t.Fatal("removeNode removed nonexistent node")
	}
}

func TestRandomNode(t *testing.T) {
	g := newTestingGateway("TestRemoveNode", t)
	defer g.Close()
	id := g.mu.RLock()

	if addr, err := g.randomNode(); err != nil {
		t.Fatal("randomNode failed:", err)
	} else if addr != g.Address() {
		t.Fatal("randomNode returned wrong address:", addr)
	}
	g.mu.RUnlock(id)

	id = g.mu.Lock()
	g.removeNode(g.myAddr)
	if _, err := g.randomNode(); err != errNoPeers {
		t.Fatalf("randomNode returned wrong error: expected %v, got %v", errNoPeers, err)
	}

	nodes := map[modules.NetAddress]int{
		"foo": 0,
		"bar": 0,
		"baz": 0,
	}
	for addr := range nodes {
		g.addNode(addr)
	}
	g.mu.Unlock(id)

	for i := 0; i < len(nodes)*10; i++ {
		id = g.mu.RLock()
		addr, err := g.randomNode()
		g.mu.RUnlock(id)
		if err != nil {
			t.Fatal("randomNode failed:", err)
		}
		nodes[addr]++
	}
	for node, count := range nodes {
		if count == 0 { // 1-in-200000 chance of occuring naturally
			t.Errorf("node %v was never selected", node, count)
		}
	}
}

func TestShareNodes(t *testing.T) {
	g1 := newTestingGateway("TestShareNodes1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestShareNodes2", t)
	defer g2.Close()

	// add a node to g2
	g2.addNode("foo")

	// connect
	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("couldn't connect:", err)
	}

	// g1 should have received foo
	if g1.addNode("foo") == nil {
		t.Fatal("gateway did not receive nodes during Connect:", g1.nodes)
	}

	// remove all nodes from both peers
	g1.nodes = map[modules.NetAddress]struct{}{}
	g2.nodes = map[modules.NetAddress]struct{}{}

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
	for i := 0; i < maxSharedNodes+10; i++ {
		g2.addNode(modules.NetAddress("foo" + strconv.Itoa(i)))
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

func TestRelayNodes(t *testing.T) {
	g1 := newTestingGateway("TestRelayNodes1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestRelayNodes2", t)
	defer g2.Close()
	g3 := newTestingGateway("TestRelayNodes3", t)
	defer g2.Close()

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

	// g2 should have received g3's address from g1
	time.Sleep(100 * time.Millisecond)
	id := g2.mu.RLock()
	defer g2.mu.RUnlock(id)
	if _, ok := g2.nodes[g3.Address()]; !ok {
		t.Fatal("node was not relayed:", g2.nodes)
	}
}
