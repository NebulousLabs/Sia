package api

import (
	"io/ioutil"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules/renter"
)

// createRandFile creates a file on disk and fills it with random bytes.
func createRandFile(path string) error {
	data, err := crypto.RandBytes(1024)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0600)
}

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
	path := filepath.Join(build.SiaTestingDir, "api", "TestRenterPaths", "test.dat")
	err = createRandFile(path)
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

	// File should be listed by the renter.
	var rf RenterFiles
	err = st.getAPI("/renter/files", &rf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Files) != 1 || rf.Files[0].Nickname != "foo/bar/test" {
		t.Fatal("/renter/files did not return correct file:", rf)
	}
}

// TestRenterConflicts tests that the renter handles naming conflicts properly.
func TestRenterConflicts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestRenterConflicts")
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
	path := filepath.Join(build.SiaTestingDir, "api", "TestRenterConflicts", "test.dat")
	err = createRandFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host, using a path designed to cause conflicts. The renter
	// should automatically create a folder called foo/bar.sia. Later, we'll
	// exploit this by uploading a file called foo/bar.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/foo/bar.sia/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// File should be listed by the renter.
	var rf RenterFiles
	err = st.getAPI("/renter/files", &rf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Files) != 1 || rf.Files[0].Nickname != "foo/bar.sia/test" {
		t.Fatal("/renter/files did not return correct file:", rf)
	}

	// Upload using the same nickname.
	err = st.stdPostAPI("/renter/upload/foo/bar.sia/test", uploadValues)
	if err == renter.ErrDuplicateNickname {
		t.Fatalf("expected %v, got %v", renter.ErrDuplicateNickname, err)
	}

	// Upload using nickname that conflicts with folder.
	err = st.stdPostAPI("/renter/upload/foo/bar", uploadValues)
	if err == nil {
		t.Fatal("expecting conflict error, got nil")
	}
}
