package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// TestStartDaemon probes the startDaemon function.
func TestStartDaemon(t *testing.T) {
	testDir := build.TempDir("siad", "TestStartDaemon")
	config.Siad.NoBootstrap = false
	config.Siad.APIaddr = "localhost:45170"
	config.Siad.RPCaddr = ":45171"
	config.Siad.HostAddr = ":45172"
	config.Siad.SiaDir = testDir
	go func() {
		err := startDaemon()
		if err != nil {
			t.Error(err)
		}
	}()

	// Wait until the server has started, and then send a kill command to the
	// daemon.
	<-started
	time.Sleep(250 * time.Millisecond)
	resp, err := http.Get("http://localhost:45170/daemon/stop")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	resp.Body.Close()
}
