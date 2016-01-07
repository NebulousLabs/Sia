package api

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
)

// TestRenterPaths tests that the /renter routes handle path parameters
// properly.
func TestRenterPaths(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestRenterPaths")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	stDir := filepath.Join(build.SiaTestingDir, "api", "TestRenterPaths")
	path := filepath.Join(stDir, "test.dat")
	data, err := crypto.RandBytes(1024)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(path, data, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/foo/bar/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// Renter should have created foo/bar structure.
	// (See modules/renter/persist_test.go for details on this test)
	var walkStr string
	filepath.Walk(stDir, func(path string, _ os.FileInfo, _ error) error {
		if filepath.Ext(path) == ".sia" {
			rel, _ := filepath.Rel(stDir, path)
			walkStr += rel
		}
		return nil
	})
	expWalkStr := "renter/foo/bar/test.sia"
	if walkStr != expWalkStr {
		t.Fatalf("Bad walk string: expected %v, got %v", expWalkStr, walkStr)
	}
}
