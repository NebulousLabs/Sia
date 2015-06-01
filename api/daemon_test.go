package api

import (
	"fmt"
	"net/http"
	"testing"
)

// TestNewerVersion checks that in all cases, newerVesion returns the correct
// result.
func TestNewerVersion(t *testing.T) {
	// If the VERSION is changed, these tests might no longer be valid.
	if VERSION != "0.3.2" {
		t.Fatal("Need to update version tests")
	}

	versionMap := map[string]bool{
		VERSION:   false,
		"0.1":     false,
		"0.1.1":   false,
		"1":       true,
		"0.9":     true,
		"0.3.1.9": false,
		"0.3.2.0": true,
		"0.3.2.1": true,
	}

	for version, expected := range versionMap {
		if newerVersion(version) != expected {
			t.Errorf("Comparing %v to %v should return %v", version, VERSION, expected)
		}
	}
}

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
	st := newServerTester("TestSignedUpdate", t)

	// to test the update process, we need to spoof the update server
	uh := new(updateHandler)
	http.Handle("/", uh)
	go http.ListenAndServe(":8080", nil)
	updateURL = "http://localhost:8080"

	// same version
	uh.version = VERSION
	var info UpdateInfo
	st.getAPI("/daemon/updates/check", &info)
	if info.Available {
		t.Error("new version should not be available")
	}

	// newer version
	uh.version = "0.4"
	st.getAPI("/daemon/updates/check", &info)
	if !info.Available {
		t.Error("new version should be available")
	}

	// apply (bad signature)
	resp, err := http.Get("http://localhost" + st.server.apiServer.Addr + "/daemon/updates/apply?version=current")
	if err != nil {
		t.Fatal("GET failed:", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Error("expected internal server error, got", resp.StatusCode)
	}
}
