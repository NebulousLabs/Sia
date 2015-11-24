package api

import (
	"testing"
)

// TestExplorerPreset checks that the default configuration for the explorer is
// working correctly.
func TestExplorerPreset(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createExplorerServerTester("TestExplorerPreset")
	if err != nil {
		t.Fatal(err)
	}

	// Try calling an illegal api endpoint.
	err = st.stdGetAPI("/daemon/stop")
	if err == nil {
		t.Fatal("/daemon/stop should be protected")
	}

	// Try calling a legal endpoint without a user agent.
	err = st.stdGetAPIUA("/explorer", "")
	if err != nil {
		t.Fatal(err)
	}
}

// TestReloading reloads a server and does smoke testing to see that modules
// are still working after reload.
func TestReloading(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a server tester, which will have blocks mined. Then get the
	// reloaded version of the server tester (all persistence files get copied
	// to a new folder, and then the modules are pointed at the new folders
	// during calls to 'New')
	st, err := createServerTester("TestReloading")
	if err != nil {
		t.Fatal(err)
	}
	rst, err := st.reloadedServerTester()
	if err != nil {
		t.Fatal(err)
	}
	if st.server.cs.Height() != rst.server.cs.Height() {
		t.Error("server heights do not match")
	}

	// Mine some blocks on the reloaded server and see if any errors or panics
	// are triggered.
	for i := 0; i < 3; i++ {
		_, err := rst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
}
