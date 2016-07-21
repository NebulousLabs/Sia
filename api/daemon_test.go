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
		// Skip test if the GitHub API call fails, since this seems to be a bug
		// with Travis Darwin.
		if strings.HasSuffix(err.Error(), errEmptyUpdateResponse.Error()) {
			t.Skip(err)
		}
		// Skip test if testing machine is offline.
		if strings.HasSuffix(err.Error(), "no such host") {
			t.Skipf("skipping since testing machine appears to be offline; call to /daemon/update failed with error %v", err)
		}
		t.Fatal(err)
	}
	// The /daemon/update call should report that the version is up to date and
	// no update is available.
	if update.Version != build.Version {
		// It would be especially worrisome if the call were to report that the
		// version was not up to date but no update was available.
		if !update.Available {
			t.Fatal("the /daemon/update API call is reporting a version that is not up to date but says no update is available")
		}
		t.Fatal("the /daemon/update API call is reporting that the daemon needs an update")
	}
	// It's also particularly concerning if the version is up to date but an
	// update is somehow available.
	if update.Available {
		t.Fatal("the /daemon/update API call is reporting an up to date version but says an update is available")
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
