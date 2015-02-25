package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestEntryWeight submits multiple entries to get weighed and checks that the
// results match the expected.
func TestEntryWeight(t *testing.T) {
	// Create a normal host entry and check that the weight comes back as the
	// expected value.
	normalEntry := modules.HostEntry{
		HostSettings: modules.HostSettings{
			Price:      currencyTen,
			Collateral: currencyTen,
		},
	}
	expectedWeight := baseWeight.Mul(currencyTen).Div(currencyTen).Div(currencyTen)
	if expectedWeight.Cmp(entryWeight(normalEntry)) != 0 {
		t.Error("unexpected weight for normal host entry")
	}

	// Create a collateral heavy host entry and check that the weight comes
	// back as the expected value.
	heavyCollateralEntry := modules.HostEntry{
		HostSettings: modules.HostSettings{
			Price:      currencyTen,
			Collateral: currencyThousand,
		},
	}
	expectedWeight = baseWeight.Mul(currencyTwenty).Div(currencyTen).Div(currencyTen)
	if expectedWeight.Cmp(entryWeight(heavyCollateralEntry)) != 0 {
		t.Error("unexpected weight for heavy host entry")
	}

	// Create a collateral light host entry and check that the weight comes
	// back as the expected value.
	lightCollateralEntry := modules.HostEntry{
		HostSettings: modules.HostSettings{
			Price:      currencyTen,
			Collateral: currencyTwo,
		},
	}
	expectedWeight = baseWeight.Mul(currencyFive).Div(currencyTen).Div(currencyTen)
	if expectedWeight.Cmp(entryWeight(lightCollateralEntry)) != 0 {
		t.Error("unexpected weight for light host entry")
	}
}

// TestInsertAllHosts inserts a few items into the hostdb and checks that
// allHosts updates as expected.
func TestInsertAllHosts(t *testing.T) {
	hdbt := CreateHostDBTester(t)

	// Insert a standard entry using the exported function.
	normalEntry := modules.HostEntry{
		IPAddress: ":2501",
	}
	err := hdbt.Insert(normalEntry)
	if err != nil {
		t.Fatal(err)
	}
	if len(hdbt.allHosts) != 1 {
		t.Error("expecting 1 host in allHosts, got", len(hdbt.allHosts))
	}

	// Insert the entry again, the size of allHosts should not change.
	err = hdbt.Insert(normalEntry)
	if err != nil {
		t.Fatal(err)
	}
	if len(hdbt.allHosts) != 1 {
		t.Error("expecting 1 host in allHosts, got", len(hdbt.allHosts))
	}

	// Insert an entry that has the same host but a different port. allHosts
	// should increase to 2 total in size.
	sameHostEntry := modules.HostEntry{
		IPAddress: ":2502",
	}
	err = hdbt.Insert(sameHostEntry)
	if err != nil {
		t.Fatal(err)
	}
	if len(hdbt.allHosts) != 2 {
		t.Error("expecting 2 hosts in allHosts, got", len(hdbt.allHosts))
	}
}
