package api

import (
	"io/ioutil"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationHosting tests that the host correctly receives payment for
// hosting files.
func TestIntegrationHosting(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestIntegrationHosting")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host.
	announceValues := url.Values{}
	announceValues.Set("address", string(st.host.NetAddress()))
	err = st.stdPostAPI("/host/announce", announceValues)
	if err != nil {
		t.Fatal(err)
	}

	// mine block and wait for announcement to register
	st.miner.AddBlock()
	var hosts ActiveHosts
	time.Sleep(1 * time.Second)
	st.getAPI("/hostdb/hosts/active", &hosts)
	if len(hosts.Hosts) == 0 {
		t.Fatal("host announcement not seen")
	}

	// create a file
	path := filepath.Join(build.SiaTestingDir, "api", "TestIntegrationHosting", "test.dat")
	data, err := crypto.RandBytes(1024)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(path, data, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// upload to host
	err = st.stdGetAPI("/renter/files/upload?nickname=test&duration=10&source=" + path)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	var fi []FileInfo
	for len(fi) != 1 || fi[0].UploadProgress != 10 {
		time.Sleep(3 * time.Second)
		st.getAPI("/renter/files/list", &fi)
	}

	// mine blocks until storage proof is complete
	for i := 0; i < 20+int(types.MaturityDelay); i++ {
		st.miner.AddBlock()
	}

	// check profit
	var hg HostGET
	err = st.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}
	expRevenue := "31857777777777575235"
	if hg.Revenue.String() != expRevenue {
		t.Fatalf("host's profit was not affected: expected %v, got %v", expRevenue, hg.Revenue)
	}
}
