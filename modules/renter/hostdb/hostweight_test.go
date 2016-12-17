package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

func calculateWeightFromUInt64Price(price uint64) (weight types.Currency) {
	hdb := bareHostDB()
	hdb.blockHeight = 0
	var entry hostEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(price)
	return hdb.calculateHostWeight()(entry.HostDBEntry)
}

func TestHostWeightDistinctPrices(t *testing.T) {
	// Create two identical entries, except that one has a price that is 2x the
	// other. The weight returned by hostWeight should be 1/8 for the more
	// expensive host.
	weight1 := calculateWeightFromUInt64Price(3)
	weight2 := calculateWeightFromUInt64Price(6)
	expectedWeight := weight1.Div64(32)
	if weight2.Cmp(expectedWeight) != 0 {
		t.Error("Weight of expensive host is not the correct value.")
	}
}

func TestHostWeightIdenticalPrices(t *testing.T) {
	weight1 := calculateWeightFromUInt64Price(42)
	weight2 := calculateWeightFromUInt64Price(42)
	if weight1.Cmp(weight2) != 0 {
		t.Error("Weight of identically priced hosts should be equal.")
	}
}

func TestHostWeightWithOnePricedZero(t *testing.T) {
	weight1 := calculateWeightFromUInt64Price(5)
	weight2 := calculateWeightFromUInt64Price(0)
	if weight1.Cmp(weight2) >= 0 {
		t.Error("Zero-priced host should have higher weight than nonzero-priced host.")
	}
}

func TestHostWeightWithBothPricesZero(t *testing.T) {
	weight1 := calculateWeightFromUInt64Price(0)
	weight2 := calculateWeightFromUInt64Price(0)
	if weight1.Cmp(weight2) != 0 {
		t.Error("Weight of two zero-priced hosts should be equal.")
	}
}
