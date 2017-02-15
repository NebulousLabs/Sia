package api

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

// TestHostDBHostsActiveHandler checks the behavior of the call to
// /hostdb/active.
func TestHostDBHostsActiveHandler(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostDBHostsActiveHandler")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Try the call with numhosts unset, and set to -1, 0, and 1.
	var ah HostdbActiveGET
	err = st.getAPI("/hostdb/active", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=-1", &ah)
	if err == nil {
		t.Fatal("expecting an error, got:", err)
	}
	err = st.getAPI("/hostdb/active?numhosts=0", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=1", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}

	// announce the host and start accepting contracts
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	err = st.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = st.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}

	// Try the call with with numhosts unset, and set to -1, 0, 1, and 2.
	err = st.getAPI("/hostdb/active", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=-1", &ah)
	if err == nil {
		t.Fatal("expecting an error, got:", err)
	}
	err = st.getAPI("/hostdb/active?numhosts=0", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=1", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=2", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatal(len(ah.Hosts))
	}
}

// TestHostDBHostsAllHandler checks that announcing a host adds it to the list
// of all hosts.
func TestHostDBHostsAllHandler(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostDBHostsAllHandler")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Try the call before any hosts have been declared.
	var ah HostdbAllGET
	if err = st.getAPI("/hostdb/all", &ah); err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatalf("expected 0 hosts, got %v", len(ah.Hosts))
	}
	// Announce the host and try the call again.
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/hostdb/all", &ah); err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}
}

// TestHostDBHostsHandler checks that the hosts handler is easily able to return
func TestHostDBHostsHandler(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostDBHostsHandler")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host and then get the list of hosts.
	var ah HostdbActiveGET
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/hostdb/active", &ah); err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}

	// Parse the pubkey from the returned list of hosts and use it to form a
	// request regarding the specific host.
	keyString := ah.Hosts[0].PublicKey.String()
	if keyString != ah.Hosts[0].PublicKeyString {
		t.Error("actual key string and provided string do not match")
	}
	query := fmt.Sprintf("/hostdb/hosts/%s", ah.Hosts[0].PublicKeyString)

	// Get the detailed info for the host.
	var hh HostdbHostsGET
	if err = st.getAPI(query, &hh); err != nil {
		t.Fatal(err)
	}

	// Check that none of the values equal zero. A value of zero indicates that
	// the field is no longer being tracked/reported, which could break
	// compatibility for some apps. The default needs to be '1', not zero.
	if hh.ScoreBreakdown.AgeAdjustment == 0 {
		t.Error("Zero value in host score breakdown")
	}
	if hh.ScoreBreakdown.BurnAdjustment == 0 {
		t.Error("Zero value in host score breakdown")
	}
	if hh.ScoreBreakdown.CollateralAdjustment == 0 {
		t.Error("Zero value in host score breakdown")
	}
	if hh.ScoreBreakdown.PriceAdjustment == 0 {
		t.Error("Zero value in host score breakdown")
	}
	if hh.ScoreBreakdown.StorageRemainingAdjustment == 0 {
		t.Error("Zero value in host score breakdown")
	}
	if hh.ScoreBreakdown.UptimeAdjustment == 0 {
		t.Error("Zero value in host score breakdown")
	}
	if hh.ScoreBreakdown.VersionAdjustment == 0 {
		t.Error("Zero value in host score breakdown")
	}

	// Check that none of the supported values equals 1. A value of 1 indicates
	// that the hostdb is not performing any penalties or rewards for that
	// field, meaning that the calibration for that field is probably incorrect.
	if hh.ScoreBreakdown.AgeAdjustment == 1 {
		t.Error("One value in host score breakdown")
	}
	// Burn adjustment is not yet supported.
	//
	// if hh.ScoreBreakdown.BurnAdjustment == 1 {
	//	t.Error("One value in host score breakdown")
	// }
	if hh.ScoreBreakdown.CollateralAdjustment == 1 {
		t.Error("One value in host score breakdown")
	}
	if hh.ScoreBreakdown.PriceAdjustment == 1 {
		t.Error("One value in host score breakdown")
	}
	if hh.ScoreBreakdown.StorageRemainingAdjustment == 1 {
		t.Error("One value in host score breakdown")
	}
	if hh.ScoreBreakdown.UptimeAdjustment == 1 {
		t.Error("One value in host score breakdown")
	}
	if hh.ScoreBreakdown.VersionAdjustment == 1 {
		t.Error("One value in host score breakdown")
	}
}

// TestHostDBAndRenterDownloadDynamicIPs checks that the hostdb and the renter are
// successfully able to follow a host that has changed IP addresses and then
// re-announced.
func TestHostDBAndRenterDownloadDynamicIPs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostDBAndRenterDownloadDynamicIPs")
	if err != nil {
		t.Fatal(err)
	}
	stHost, err := blankServerTester("TestHostDBAndRenterDownloadDynamicIPs-Host")
	if err != nil {
		t.Fatal(err)
	}
	sts := []*serverTester{st, stHost}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Announce the host.
	err = stHost.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Pull the host's net address and pubkey from the hostdb.
	var ah HostdbActiveGET
	for i := 0; i < 50; i++ {
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			t.Fatal(err)
		}
		if len(ah.Hosts) == 1 {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}
	addr := ah.Hosts[0].NetAddress
	pks := ah.Hosts[0].PublicKeyString

	// Upload a file to the host.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "10"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}
	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Upload the file to the renter.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// Try downloading the file.
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	download, err := ioutil.ReadFile(downpath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig, download) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}

	// Close and re-open the host. This should reset the host's address, as the
	// host should now be on a new port.
	err = stHost.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	stHost, err = assembleServerTester(stHost.walletKey, stHost.dir)
	if err != nil {
		t.Fatal(err)
	}
	sts[1] = stHost
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	// Pull the host's net address and pubkey from the hostdb.
	err = retry(50, time.Millisecond*100, func() error {
		// Get the hostdb internals.
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			return err
		}

		// Get the host's internals.
		var hg HostGET
		if err = stHost.getAPI("/host", &hg); err != nil {
			return err
		}

		if len(ah.Hosts) != 1 {
			return fmt.Errorf("expected 1 host, got %v", len(ah.Hosts))
		}
		if ah.Hosts[0].NetAddress != hg.ExternalSettings.NetAddress {
			return fmt.Errorf("hostdb net address doesn't match host net address: %v : %v", ah.Hosts[0].NetAddress, hg.ExternalSettings.NetAddress)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ah.Hosts[0].PublicKeyString != pks {
		t.Error("public key appears to have changed for host")
	}
	if ah.Hosts[0].NetAddress == addr {
		t.Log("NetAddress did not change for the new host")
	}

	// Try downloading the file.
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	download, err = ioutil.ReadFile(downpath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig, download) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}
}

// TestHostDBAndRenterUploadDynamicIPs checks that the hostdb and the renter are
// successfully able to follow a host that has changed IP addresses and then
// re-announced.
func TestHostDBAndRenterUploadDynamicIPs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostDBAndRenterUploadDynamicIPs")
	if err != nil {
		t.Fatal(err)
	}
	stHost, err := blankServerTester("TestHostDBAndRenterUploadDynamicIPs-Host")
	if err != nil {
		t.Fatal(err)
	}
	sts := []*serverTester{st, stHost}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Announce the host.
	err = stHost.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Pull the host's net address and pubkey from the hostdb.
	var ah HostdbActiveGET
	for i := 0; i < 50; i++ {
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			t.Fatal(err)
		}
		if len(ah.Hosts) == 1 {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}
	addr := ah.Hosts[0].NetAddress
	pks := ah.Hosts[0].PublicKeyString

	// Upload a file to the host.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "10"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}
	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Upload the file to the renter.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// Close and re-open the host. This should reset the host's address, as the
	// host should now be on a new port.
	err = stHost.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	stHost, err = assembleServerTester(stHost.walletKey, stHost.dir)
	if err != nil {
		t.Fatal(err)
	}
	sts[1] = stHost
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	// Pull the host's net address and pubkey from the hostdb.
	err = retry(50, time.Millisecond*100, func() error {
		// Get the hostdb internals.
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			return err
		}

		// Get the host's internals.
		var hg HostGET
		if err = stHost.getAPI("/host", &hg); err != nil {
			return err
		}

		if len(ah.Hosts) != 1 {
			return fmt.Errorf("expected 1 host, got %v", len(ah.Hosts))
		}
		if ah.Hosts[0].NetAddress != hg.ExternalSettings.NetAddress {
			return fmt.Errorf("hostdb net address doesn't match host net address: %v : %v", ah.Hosts[0].NetAddress, hg.ExternalSettings.NetAddress)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ah.Hosts[0].PublicKeyString != pks {
		t.Error("public key appears to have changed for host")
	}
	if ah.Hosts[0].NetAddress == addr {
		t.Log("NetAddress did not change for the new host")
	}

	// Try uploading a second file.
	path2 := filepath.Join(st.dir, "test2.dat")
	test2Size := modules.SectorSize*2 + 1
	err = createRandFile(path2, int(test2Size))
	if err != nil {
		t.Fatal(err)
	}
	uploadValues = url.Values{}
	uploadValues.Set("source", path2)
	err = st.stdPostAPI("/renter/upload/test2", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
	for i := 0; i < 200 && (len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0], rf.Files[1])
	}

	// Try downloading the second file.
	downpath2 := filepath.Join(st.dir, "testdown2.dat")
	err = st.stdGetAPI("/renter/download/test2?destination=" + downpath2)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig2, err := ioutil.ReadFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	download2, err := ioutil.ReadFile(downpath2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig2, download2) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}
}

// TestHostDBAndRenterFormDynamicIPs checks that the hostdb and the renter are
// successfully able to follow a host that has changed IP addresses and then
// re-announced.
func TestHostDBAndRenterFormDynamicIPs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostDBAndRenterFormDynamicIPs")
	if err != nil {
		t.Fatal(err)
	}
	stHost, err := blankServerTester("TestHostDBAndRenterFormDynamicIPs-Host")
	if err != nil {
		t.Fatal(err)
	}
	sts := []*serverTester{st, stHost}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Announce the host.
	err = stHost.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Pull the host's net address and pubkey from the hostdb.
	var ah HostdbActiveGET
	for i := 0; i < 50; i++ {
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			t.Fatal(err)
		}
		if len(ah.Hosts) == 1 {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}
	addr := ah.Hosts[0].NetAddress
	pks := ah.Hosts[0].PublicKeyString

	// Close and re-open the host. This should reset the host's address, as the
	// host should now be on a new port.
	err = stHost.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	stHost, err = assembleServerTester(stHost.walletKey, stHost.dir)
	if err != nil {
		t.Fatal(err)
	}
	sts[1] = stHost
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	// Pull the host's net address and pubkey from the hostdb.
	err = retry(50, time.Millisecond*100, func() error {
		// Get the hostdb internals.
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			return err
		}

		// Get the host's internals.
		var hg HostGET
		if err = stHost.getAPI("/host", &hg); err != nil {
			return err
		}

		if len(ah.Hosts) != 1 {
			return fmt.Errorf("expected 1 host, got %v", len(ah.Hosts))
		}
		if ah.Hosts[0].NetAddress != hg.ExternalSettings.NetAddress {
			return fmt.Errorf("hostdb net address doesn't match host net address: %v : %v", ah.Hosts[0].NetAddress, hg.ExternalSettings.NetAddress)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ah.Hosts[0].PublicKeyString != pks {
		t.Error("public key appears to have changed for host")
	}
	if ah.Hosts[0].NetAddress == addr {
		t.Log("NetAddress did not change for the new host")
	}

	// Upload a file to the host.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "10"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}
	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Upload the file to the renter.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// Try downloading the file.
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	download, err := ioutil.ReadFile(downpath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig, download) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}
}

// TestHostDBAndRenterRenewDynamicIPs checks that the hostdb and the renter are
// successfully able to follow a host that has changed IP addresses and then
// re-announced.
func TestHostDBAndRenterRenewDynamicIPs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostDBAndRenterRenewDynamicIPs")
	if err != nil {
		t.Fatal(err)
	}
	stHost, err := blankServerTester("TestHostDBAndRenterRenewDynamicIPs-Host")
	if err != nil {
		t.Fatal(err)
	}
	sts := []*serverTester{st, stHost}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Announce the host.
	err = stHost.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	var ah HostdbActiveGET
	err = retry(50, 100*time.Millisecond, func() error {
		if err := st.getAPI("/hostdb/active", &ah); err != nil {
			return err
		}
		if len(ah.Hosts) != 1 {
			return errors.New("host not found in hostdb")
		}
		return nil
	})

	// Upload a file to the host.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "10"
	testPeriodInt := 10
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}
	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Upload the file to the renter.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// Try downloading the file.
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	download, err := ioutil.ReadFile(downpath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig, download) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}

	// Close and re-open the host. This should reset the host's address, as the
	// host should now be on a new port.
	/*
		err = stHost.server.Close()
		if err != nil {
			t.Fatal(err)
		}
		stHost, err = assembleServerTester(stHost.walletKey, stHost.dir)
		if err != nil {
			t.Fatal(err)
		}
		sts[1] = stHost
		err = fullyConnectNodes(sts)
		if err != nil {
			t.Fatal(err)
		}
		err = stHost.announceHost()
		if err != nil {
			t.Fatal(err)
		}
	*/
	// Pull the host's net address and pubkey from the hostdb.
	err = retry(50, time.Millisecond*100, func() error {
		// Get the hostdb internals.
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			return err
		}

		// Get the host's internals.
		var hg HostGET
		if err = stHost.getAPI("/host", &hg); err != nil {
			return err
		}

		if len(ah.Hosts) != 1 {
			return fmt.Errorf("expected 1 host, got %v", len(ah.Hosts))
		}
		if ah.Hosts[0].NetAddress != hg.ExternalSettings.NetAddress {
			return fmt.Errorf("hostdb net address doesn't match host net address: %v : %v", ah.Hosts[0].NetAddress, hg.ExternalSettings.NetAddress)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine enough blocks that multiple renew cylces happen. After the renewing
	// happens, the file should still be downloadable.
	for i := 0; i < testPeriodInt*2; i++ {
		_, err = st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		_, err = synchronizationCheck(sts)
		if err != nil {
			t.Fatal(err)
		}
		// Give time for the upgrade to happen.
		time.Sleep(time.Second * 3)
	}

	// Try downloading the file.
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	download, err = ioutil.ReadFile(downpath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig, download) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}
}
