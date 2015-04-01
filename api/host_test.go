package api

import (
	"testing"
	"time"
)

// announceHost puts a host announcement for the host into the blockchain.
func (dt *daemonTester) announceHost() {
	dt.callAPI("/host/announce")
	dt.mineBlock()
}

// TestHostAnnouncement checks that calling '/host/announce' results in an
// announcement that makes it into the blockchain.
func TestHostAnnouncement(t *testing.T) {
	// Create the daemon tester and check that the initial hostdb is empty.
	dt := newDaemonTester(t)
	if dt.hostdb.NumHosts() != 0 {
		t.Fatal("hostdb needs to be empty after calling newDaemonTester")
	}

	// Announce the host and check that the announcement makes it into the
	// hostdb. Processing an announcement involves network communication which
	// happens in a separate goroutine. Since there's not a good way to figure
	// out when the call will finish, we spin until the update has finished. If
	// the update never finishes, the test environment should timeout.
	dt.announceHost()
	for dt.hostdb.NumHosts() != 1 {
		time.Sleep(time.Millisecond)
	}
}
