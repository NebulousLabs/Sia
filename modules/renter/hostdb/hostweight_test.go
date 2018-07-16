package hostdb

import (
	"testing"
	"time"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"
)

func calculateWeightFromUInt64Price(price uint64) (weight types.Currency) {
	hdb := bareHostDB()
	hdb.blockHeight = 0
	var entry modules.HostDBEntry
	entry.Version = build.Version
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(price).Mul(types.SiacoinPrecision).Div64(4032).Div64(1e9)
	return hdb.calculateHostWeight(entry)
}

func TestHostWeightDistinctPrices(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	weight1 := calculateWeightFromUInt64Price(300)
	weight2 := calculateWeightFromUInt64Price(301)
	if weight1.Cmp(weight2) <= 0 {
		t.Log(weight1)
		t.Log(weight2)
		t.Error("Weight of expensive host is not the correct value.")
	}
}

func TestHostWeightIdenticalPrices(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	weight1 := calculateWeightFromUInt64Price(42)
	weight2 := calculateWeightFromUInt64Price(42)
	if weight1.Cmp(weight2) != 0 {
		t.Error("Weight of identically priced hosts should be equal.")
	}
}

func TestHostWeightWithOnePricedZero(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	weight1 := calculateWeightFromUInt64Price(5)
	weight2 := calculateWeightFromUInt64Price(0)
	if weight1.Cmp(weight2) >= 0 {
		t.Error("Zero-priced host should have higher weight than nonzero-priced host.")
	}
}

func TestHostWeightWithBothPricesZero(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	weight1 := calculateWeightFromUInt64Price(0)
	weight2 := calculateWeightFromUInt64Price(0)
	if weight1.Cmp(weight2) != 0 {
		t.Error("Weight of two zero-priced hosts should be equal.")
	}
}

func TestHostWeightCollateralDifferences(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry2 := entry
	entry2.Collateral = types.NewCurrency64(500).Mul(types.SiacoinPrecision)

	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)
	if w1.Cmp(w2) < 0 {
		t.Error("Larger collateral should have more weight")
	}
}

func TestHostWeightStorageRemainingDifferences(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)

	entry2 := entry
	entry2.RemainingStorage = 50e3
	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)

	if w1.Cmp(w2) < 0 {
		t.Error("Larger storage remaining should have more weight")
	}
}

func TestHostWeightVersionDifferences(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Version = "v1.0.4"

	entry2 := entry
	entry2.Version = "v1.0.3"
	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)

	if w1.Cmp(w2) < 0 {
		t.Error("Higher version should have more weight")
	}
}

func TestHostWeightLifetimeDifferences(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	hdb.blockHeight = 10000
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Version = "v1.0.4"

	entry2 := entry
	entry2.FirstSeen = 8100
	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)

	if w1.Cmp(w2) < 0 {
		t.Error("Been around longer should have more weight")
	}
}

func TestHostWeightUptimeDifferences(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	hdb.blockHeight = 10000
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Version = "v1.0.4"
	entry.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: true},
	}

	entry2 := entry
	entry2.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: false},
	}
	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)

	if w1.Cmp(w2) < 0 {
		t.Error("Been around longer should have more weight")
	}
}

func TestHostWeightUptimeDifferences2(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	hdb.blockHeight = 10000
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Version = "v1.0.4"
	entry.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: false},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: true},
	}

	entry2 := entry
	entry2.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: false},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: true},
	}
	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)

	if w1.Cmp(w2) < 0 {
		t.Errorf("Been around longer should have more weight\n\t%v\n\t%v", w1, w2)
	}
}

func TestHostWeightUptimeDifferences3(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	hdb.blockHeight = 10000
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Version = "v1.0.4"
	entry.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: false},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: true},
	}

	entry2 := entry
	entry2.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: false},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: false},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: true},
	}
	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)

	if w1.Cmp(w2) < 0 {
		t.Error("Been around longer should have more weight")
	}
}

func TestHostWeightUptimeDifferences4(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	hdb.blockHeight = 10000
	var entry modules.HostDBEntry
	entry.RemainingStorage = 250e3
	entry.StoragePrice = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Collateral = types.NewCurrency64(1000).Mul(types.SiacoinPrecision)
	entry.Version = "v1.0.4"
	entry.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: false},
	}

	entry2 := entry
	entry2.ScanHistory = modules.HostDBScans{
		{Timestamp: time.Now().Add(time.Hour * -100), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -80), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -60), Success: true},
		{Timestamp: time.Now().Add(time.Hour * -40), Success: false},
		{Timestamp: time.Now().Add(time.Hour * -20), Success: false},
	}
	w1 := hdb.calculateHostWeight(entry)
	w2 := hdb.calculateHostWeight(entry2)

	if w1.Cmp(w2) < 0 {
		t.Error("Been around longer should have more weight")
	}
}
