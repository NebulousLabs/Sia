package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestHostWeight probes the hostWeight function.
func TestHostWeight(t *testing.T) {
	hdbt := newHDBTester("TestHostWeight", t)

	// Create two identical entries, except that one has a price that is 2x the
	// other. The weight returned by hostWeight should be 1/8 for the more
	// expensive host.
	entry1 := hostEntry{
		HostSettings: modules.HostSettings{
			Price: types.NewCurrency64(3),
		},
	}
	entry2 := hostEntry{
		HostSettings: modules.HostSettings{
			Price: types.NewCurrency64(6),
		},
	}

	weight1 := hdbt.hostdb.hostWeight(entry1)
	weight2 := hdbt.hostdb.hostWeight(entry2)
	expectedWeight := weight1.Div(types.NewCurrency64(8))
	if weight2.Cmp(expectedWeight) != 0 {
		t.Error("Weight of expensive host is not the correct value.")
	}

	// Try a 0 price.
	entry3 := hostEntry{
		HostSettings: modules.HostSettings{
			Price: types.NewCurrency64(0),
		},
	}
	weight3 := hdbt.hostdb.hostWeight(entry3)
	if weight3.Cmp(weight1) <= 0 {
		t.Error("Free host not weighing fairly")
	}
}
