package api

import (
	"testing"
	"time"
)

// announceHost puts a host announcement for the host into the blockchain.
func (st *serverTester) announceHost() error {
	st.callAPI("/host/announce")
	b, _ := st.miner.FindBlock()
	err := st.cs.AcceptBlock(b)
	if err != nil {
		return err
	}
	return nil
}

// TestHostAnnouncement checks that calling '/host/announce' results in an
// announcement that makes it into the blockchain.
func TestHostAnnouncement(t *testing.T) {
	t.Skip("occasionally times out")

	// Create the server tester and check that the initial hostdb is empty.
	st := newServerTester("TestHostAnnouncement", t)
	if len(st.server.hostdb.ActiveHosts()) != 0 {
		t.Fatal("hostdb needs to be empty after calling newServerTester")
	}

	// Announce the host and check that the announcement makes it into the
	// hostdb. Processing an announcement involves network communication which
	// happens in a separate goroutine. Since there's not a good way to figure
	// out when the call will finish, we spin until the update has finished. If
	// the update never finishes, the test environment should timeout.
	err := st.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	for len(st.server.hostdb.ActiveHosts()) != 1 {
		time.Sleep(time.Millisecond)
	}
}

// TestStorageProofs creates a renter-host environment where the renter uploads
// to the host and then blocks are mined until the host submits a storage
// proof.
func TestStorageProofs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Skip("test not designed to work outside of modified constants - TODO")

	// Create a server and announce the host.
	st := newServerTester("TestStorageProofs", t)
	err := st.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	for len(st.server.hostdb.ActiveHosts()) == 0 {
		time.Sleep(time.Millisecond)
	}

	// Have the renter submit an upload to the host.
	uploadName := "api.go"
	st.callAPI("/renter/files/upload?pieces=1&nickname=api.go&source=" + uploadName)
	time.Sleep(time.Second * 10)

	// Mine 25 blocks - the file will expire. (special constants)
	for i := 0; i < 25; i++ {
		b, _ := st.miner.FindBlock()
		err := st.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
		t.Error("found", i)
	}

	// Mine 25 more blocks, waiting between each block. This will give the host
	// time to submit a storage proof.
	for i := 0; i < 25; i++ {
		b, _ := st.miner.FindBlock()
		err := st.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond * 50)
		t.Error("2 - found", i)
	}
}
