package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

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
