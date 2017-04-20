package gateway

import (
	"testing"
)

func TestLoad(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)

	g.mu.Lock()
	g.addNode("111.111.111.111:1111", false)
	g.addNode("222.222.222.222:2222", true)
	g.save()
	g.mu.Unlock()
	g.Close()

	g2, err := New("localhost:0", false, g.persistDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g2.nodes["111.111.111.111:1111"]; !ok {
		t.Fatal("gateway did not load old peer list:", g2.nodes)
	}
	if _, ok := g2.nodes["222.222.222.222:2222"]; !ok {
		t.Fatal("gateway did not load old peer list:", g2.nodes)
	}
}
