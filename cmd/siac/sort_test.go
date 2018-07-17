package main

import (
	"sort"
	"testing"

	"gitlab.com/NebulousLabs/Sia/node/api"
	"gitlab.com/NebulousLabs/Sia/types"
)

// TestSortByValue tests that byValue sorts contracts correctly.
func TestSortByValue(t *testing.T) {
	contracts := []api.RenterContract{
		{RenterFunds: types.NewCurrency64(1), NetAddress: "b"},
		{RenterFunds: types.NewCurrency64(4), NetAddress: "a"},
		{RenterFunds: types.NewCurrency64(2), NetAddress: "c"},
		{RenterFunds: types.NewCurrency64(5), NetAddress: "z"},
		{RenterFunds: types.NewCurrency64(2), NetAddress: "c"},
		{RenterFunds: types.NewCurrency64(0), NetAddress: "e"},
		{RenterFunds: types.NewCurrency64(2), NetAddress: "a"},
	}
	sort.Sort(byValue(contracts))

	// check ordering
	for i := 0; i < len(contracts)-1; i++ {
		a, b := contracts[i], contracts[i+1]
		if cmp := a.RenterFunds.Cmp(b.RenterFunds); cmp < 0 {
			t.Error("contracts not primarily sorted by value:", a.RenterFunds, b.RenterFunds)
		} else if cmp == 0 && a.NetAddress > b.NetAddress {
			t.Error("contracts not secondarily sorted by address:", a.NetAddress, b.NetAddress)
		}
	}
}
