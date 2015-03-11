package hostdb

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

var (
	rpcPort int = 9700
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
	g, err := gateway.New(":"+strconv.Itoa(rpcPort), ct.State, "")
	if err != nil {
		t.Fatal(err)
	}
	rpcPort++
	hdb, err := New(ct.State, g)
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
	_, err := New(nil, nil)
	if err != ErrNilState {
		t.Error("expecting ErrNilState, got:", err)
	}
}
