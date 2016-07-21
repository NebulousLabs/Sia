package api

import (
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestVersion checks that /daemon/version is responding with the correct
// version.
func TestVersion(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestVersion")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	var dv DaemonVersion
	st.getAPI("/daemon/version", &dv)
	if dv.Version != build.Version {
		t.Fatalf("/daemon/version reporting bad version: expected %v, got %v", build.Version, dv.Version)
	}
}

// TestUpdate checks that /daemon/update correctly asserts that an update is
// not available for the daemon (since the test build is always up to date).
func TestUpdate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestUpdate")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	var update UpdateInfo
	if err = st.getAPI("/daemon/update", &update); err != nil {
		// Notify tester that the API call failed, but allow testing to continue.
		// Otherwise you have to be online to run tests.
		if strings.HasSuffix(err.Error(), errEmptyUpdateResponse.Error()) {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	if update.Available && build.Version == update.Version {
		t.Fatal("daemon should not have an update available")
	}
}

/*
// TODO: enable this test again once proper daemon shutdown is implemented (shutting down modules and listener separately).
// TestStop tests the /daemon/stop handler.
func TestStop(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestStop")
	if err != nil {
		t.Fatal(err)
	}
	var success struct{ Success bool }
	err = st.getAPI("/daemon/stop", &success)
	if err != nil {
		t.Fatal(err)
	}
	// Sleep to give time for server to close, as /daemon/stop will return success
	// before Server.Close() is called.
	time.Sleep(200 * time.Millisecond)
	err = st.getAPI("/daemon/stop", &success)
	if err == nil {
		t.Fatal("after /daemon/stop, subsequent calls should fail")
	}
}
*/
