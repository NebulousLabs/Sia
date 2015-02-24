package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// A HostDBTester is a consensus tester that contains a hostdb and has
// functions to help probe the hostdb code.
type HostDBTester struct {
	*consensus.ConsensusTester
	*HostDB
}

// CreateHostDBTester initializes a hostdb tester.
func CreateHostDBTester(t *testing.T) (hdbt *HostDBTester) {
	ct := consensus.NewTestingEnvironment(t)
	hdb, err := New(ct.State)
	if err != nil {
		t.Fatal(err)
	}
	hdbt = new(HostDBTester)
	hdbt.ConsensusTester = ct
	hdbt.HostDB = hdb
	return
}

// TestNilInitialization covers the code that checks for nil variables upon
// initialization.
func TestNilInitialization(t *testing.T) {
	_, err := New(nil)
	if err != ErrNilState {
		t.Error("expecting ErrNilState, got:", err)
	}
}
