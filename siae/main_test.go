package main

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// TestMain tries running the main executable using a few different commands.
func TestMain(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	testDir := build.TempDir("siad", "TestMain")

	// Try running and closing an instance of siad.
	os.Args = []string{
		"siad",
		"-n",
		"-a",
		"localhost:45150",
		"-r",
		"localhost:45151",
		"-H",
		"localhost:45152",
		"-d",
		testDir,
	}
	go main()

	// Wait until the daemon has started and then send a kill signal to the
	// daemon.
	<-started
	time.Sleep(250 * time.Millisecond)
	resp, err := http.Get("http://localhost:45150/daemon/stop")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	resp.Body.Close()

	// Try to run the siad version command.
	os.Args = []string{
		"siad",
		"version",
	}
	main()
}
