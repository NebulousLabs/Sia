package hostdb

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/tester"
)

// A HostDBTester is a consensus tester that contains a hostdb and has
// functions to help probe the hostdb code.
type HostDBTester struct {
	*consensus.ConsensusTester
	*HostDB
}

// CreateHostDBTester initializes a hostdb tester.
func CreateHostDBTester(directory string, t *testing.T) (hdbt *HostDBTester) {
	ct := consensus.NewTestingEnvironment(t)
	gDir := filepath.Join(tester.TempDir(directory), modules.GatewayDir)
	g, err := gateway.New(":0", ct.State, gDir)
	if err != nil {
		t.Fatal(err)
	}

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
