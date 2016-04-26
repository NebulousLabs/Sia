package api

import (
	"net/http"
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
	defer st.server.Close()

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
	defer st.server.Close()
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

// TestAuthenticated tests creating a server that requires authenticated API
// calls, and then makes (un)authenticated API calls to test the
// authentication.
func TestAuthentication(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createAuthenticatedServerTester("TestAuthentication", "password")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	testGETURL := "http://" + st.server.listener.Addr().String() + "/daemon/version"
	testPOSTURL := "http://" + st.server.listener.Addr().String() + "/host/announce"

	// Test that unauthenticated API calls fail.
	// GET
	resp, err := HttpGET(testGETURL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated API call succeeded on a server that requires authentication")
	}
	// POST
	resp, err = HttpPOST(testPOSTURL, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated API call succeeded on a server that requires authentication")
	}

	// Test that authenticated API calls with the wrong password fail.
	// GET
	resp, err = HttpGETAuthenticated(testGETURL, "wrong password")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("authenticated API call succeeded with an incorrect password")
	}
	// POST
	resp, err = HttpPOSTAuthenticated(testPOSTURL, "", "wrong password")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("authenticated API call succeeded with an incorrect password")
	}

	// Test that authenticated API calls with the correct password succeed.
	// GET
	resp, err = HttpGETAuthenticated(testGETURL, "password")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal("authenticated API call failed with the correct password")
	}
	// POST
	resp, err = HttpPOSTAuthenticated(testPOSTURL, "", "password")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatal("authenticated API call failed with the correct password")
	}
}
