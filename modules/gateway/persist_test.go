package gateway

import (
	"testing"
)

func TestLoad(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestLoad", t)
	g.mu.Lock()
	g.addNode(dummyNode)
	g.save()
	g.mu.Unlock()
	g.Close()

	g2, err := New("localhost:0", g.persistDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g2.nodes[dummyNode]; !ok {
		t.Fatal("gateway did not load old peer list:", g2.nodes)
	}
}
