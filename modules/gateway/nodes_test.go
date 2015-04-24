package gateway

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

func TestAddNode(t *testing.T) {
	g := newTestingGateway("TestAddNode", t)
	defer g.Close()
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

	if addr, err := g.randomNode(); err != nil {
		t.Fatal("randomNode failed:", err)
	} else if addr != g.Address() {
		t.Fatal("randomNode returned wrong address:", addr)
	}

	g.removeNode(g.Address())

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
	for i := 0; i < len(nodes)*10; i++ {
		addr, err := g.randomNode()
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

func TestRequestNodes(t *testing.T) {
	g1 := newTestingGateway("TestRequestNodes1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestRequestNodes2", t)
	defer g2.Close()
	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("couldn't connect:", err)
	}

	// ask gateway for nodes
	nodes, err := g1.requestNodes(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	// response should be exactly []NetAddress{g1.Address(), g2.Address()}
	if len(nodes) != 2 || (nodes[0] != g1.Address() && nodes[1] != g1.Address()) {
		t.Fatalf("gateway gave bad node list %v (expected %v)", nodes, []modules.NetAddress{g1.Address(), g2.Address()})
	}

	// add a couple more nodes
	g2.addNode("foo:9001")
	g2.addNode("bar:9002")
	g2.addNode("baz:9003")
	nodes, err = g1.requestNodes(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	// nodes should now contain 4 distinct addresses
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i] == nodes[j] {
				t.Fatal("gateway gave duplicate addresses:", nodes)
			}
		}
	}

	// remove all the nodes
	g2.removeNode("foo:9001")
	g2.removeNode("bar:9002")
	g2.removeNode("baz:9003")
	g2.removeNode(g1.Address())
	g2.removeNode(g2.Address())
	if len(g2.nodes) != 0 {
		t.Fatal("gateway has nodes remaining after removal:", g2.nodes)
	}

	// no nodes should be returned
	nodes, err = g1.requestNodes(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatal("gateway gave non-existent addresses:", nodes)
	}

	if testing.Short() {
		t.SkipNow()
	}

	// sharing should be capped at maxSharedNodes
	for i := 0; i < maxSharedNodes+10; i++ {
		g2.addNode(modules.NetAddress("foo" + strconv.Itoa(i)))
	}
	nodes, err = g1.requestNodes(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != maxSharedNodes {
		t.Fatal("gateway gave wrong number of nodes: expected %v, got %v", maxSharedNodes, len(nodes))
	}
}
