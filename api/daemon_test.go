package api

import (
	"testing"
	"time"

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
