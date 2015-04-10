package api

import (
	"testing"
	"time"
)

// announceHost puts a host announcement for the host into the blockchain.
func (st *serverTester) announceHost() {
	st.callAPI("/host/announce")
	st.mineBlock()
}

// TestHostAnnouncement checks that calling '/host/announce' results in an
// announcement that makes it into the blockchain.
func TestHostAnnouncement(t *testing.T) {
	// Create the server tester and check that the initial hostdb is empty.
	st := newServerTester("TestHostAnnouncement", t)
	if len(st.hostdb.ActiveHosts()) != 0 {
		t.Fatal("hostdb needs to be empty after calling newServerTester")
	}

	// Announce the host and check that the announcement makes it into the
	// hostdb. Processing an announcement involves network communication which
	// happens in a separate goroutine. Since there's not a good way to figure
	// out when the call will finish, we spin until the update has finished. If
	// the update never finishes, the test environment should timeout.
	st.announceHost()
	for len(st.hostdb.ActiveHosts()) != 1 {
		time.Sleep(time.Millisecond)
	}
}
