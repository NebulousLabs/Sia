package api

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

type updateHandler struct {
	version string
}

func (uh *updateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.String() {
	case "/current/MANIFEST":
		fmt.Fprintf(w, "%s\nsiad\n", uh.version)
	case "/current/siad":
		fmt.Fprint(w, "yep this is siad")
	case "/current/siad.sig":
		fmt.Fprint(w, "and this is totally a signature")
	default:
		http.NotFound(w, r)
	}
}

// TestUpdate checks that updates work properly.
func TestSignedUpdate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestSignedUpdate")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// to test the update process, we need to spoof the update server
	uh := new(updateHandler)
	http.Handle("/", uh)
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go http.Serve(l, nil)
	updateURL = "http://" + l.Addr().String()

	// same version
	uh.version = build.Version
	var info UpdateInfo
	st.getAPI("/daemon/updates/check", &info)
	if info.Available {
		t.Error("new version should not be available")
	}

	// newer version
	uh.version = "100.0"
	err = st.getAPI("/daemon/updates/check", &info)
	if err != nil {
		t.Error(err)
	} else if !info.Available {
		t.Error("new version should be available")
	}

	// apply (bad signature)
	resp, err := HttpGET("http://" + st.server.listener.Addr().String() + "/daemon/updates/apply?version=current")
	if err != nil {
		t.Fatal("GET failed:", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Error("expected internal server error, got", resp.StatusCode)
	}
}

func TestVersion(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestSignedUpdate")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	var version string
	st.getAPI("/daemon/version", &version)
	if version != build.Version {
		t.Fatalf("/daemon/version reporting bad version: expected %v, got %v", build.Version, version)
	}
}
