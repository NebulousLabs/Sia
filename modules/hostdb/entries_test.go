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
