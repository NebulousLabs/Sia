package gateway

import (
	"testing"
)

func TestLoad(t *testing.T) {
	g := newTestingGateway("TestLoad", t)
	id := g.mu.Lock()
	g.addNode(dummyNode)
	g.save()
	g.mu.Unlock(id)
	g.Close()

	g2, err := New(":0", g.persistDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g2.nodes[dummyNode]; !ok {
		t.Fatal("gateway did not load old peer list:", g2.nodes)
	}
}
