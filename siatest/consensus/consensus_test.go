package main

import (
	"testing"

	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
)

// TestApiHeight checks if the consensus api endpoint works
func TestApiHeight(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testdir, err := siatest.TestDir(siatest.SiaTestingDir, t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a new server
	c, s, err := siatest.NewClientServerPair(node.AllModules(testdir))
	if err != nil {
		t.Fatal(err)
	}

	// Send GET request
	cg, err := c.GetConsensus()
	if err != nil {
		t.Fatal(err)
	}

	// Check height
	height := cg.Height

	// Mine a block
	if err := s.MineBlock(); err != nil {
		t.Fatal(err)
	}

	// Check height again
	if cg.Height != height+1 {
		t.Fatal("Height should have increased by 1 block")
	}

	// Close the server and check error
	defer func() {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}()
}
