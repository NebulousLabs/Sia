package api

import (
	"fmt"
	"testing"
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
	st, err := createServerTester("TestHostDBHostsAllHandler")
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
	query := fmt.Sprintf("/hostdb/hosts/%s", keyString)

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
