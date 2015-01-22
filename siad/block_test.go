package main

import (
	"testing"

	// "github.com/NebulousLabs/Sia/consensus"
)

func testEmptyBlock(t *testing.T, d *daemon) {
}

func TestBlockHandling(t *testing.T) {
	config := daemonTestConfig()
	d, err := newDaemon(config)
	if err != nil {
		t.Fatal(err)
	}

	testEmptyBlock(t, d)
}
