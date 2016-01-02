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
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("duration", "10")
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	var fi []FileInfo
	for len(fi) != 1 || fi[0].UploadProgress != 10 {
		st.getAPI("/renter/files", &fi)
		time.Sleep(1 * time.Second)
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
	expRevenue := "15928888888855757473"
	if hg.Revenue.String() != expRevenue {
		t.Fatalf("host's profit was not affected: expected %v, got %v", expRevenue, hg.Revenue)
	}
}

// TestIntegrationRenewing tests that the renter and host manage contract
// renewals properly.
func TestIntegrationRenewing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestIntegrationRenewing")
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
	path := filepath.Join(build.SiaTestingDir, "api", "TestIntegrationRenewing", "test.dat")
	data, err := crypto.RandBytes(1024)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(path, data, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// upload to host, specifying that the file should be renewed
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	var fi []FileInfo
	for len(fi) != 1 || fi[0].UploadProgress != 10 {
		time.Sleep(1 * time.Second)
		st.getAPI("/renter/files", &fi)
	}
	// default expiration is 60 blocks
	expExpiration := st.cs.Height() + 60
	if fi[0].Expiration != expExpiration {
		t.Fatalf("expected expiration of %v, got %v", expExpiration, fi[0].Expiration)
	}

	// mine blocks until we hit the renew threshold (default 20 blocks)
	for st.cs.Height() < expExpiration-20 {
		st.miner.AddBlock()
	}

	// renter should now renew the contract for another 60 blocks
	newExpiration := st.cs.Height() + 60
	for fi[0].Expiration != newExpiration {
		time.Sleep(1 * time.Second)
		st.getAPI("/renter/files", &fi)
	}
}
