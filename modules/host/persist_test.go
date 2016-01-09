package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestEarlySaving checks that the early host is correctly saving values to
// disk.
func TestEarlySaving(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestEarlySaving")
	if err != nil {
		t.Fatal(err)
	}

	// Store a few of the important fields.
	var oldSK crypto.SecretKey
	copy(oldSK[:], ht.host.secretKey[:])
	oldFileCounter := ht.host.fileCounter
	oldSpaceRemaining := ht.host.spaceRemaining
	oldRevenue := ht.host.revenue

	// Corrupt the fields.
	ht.host.secretKey[0]++
	ht.host.fileCounter += 7e6
	ht.host.spaceRemaining += 25e9
	ht.host.revenue = ht.host.revenue.Add(types.NewCurrency64(91e3))

	// Load the host and see that the fields are reset correctly.
	err = ht.host.load()
	if err != nil {
		t.Fatal(err)
	}
	if ht.host.secretKey != oldSK {
		t.Error("secret key not loaded correctly")
	}
	if ht.host.fileCounter != oldFileCounter {
		t.Error("file counter not loaded correctly")
	}
	if ht.host.spaceRemaining != oldSpaceRemaining {
		t.Error("space remaining not loaded correctly")
	}
	if ht.host.revenue.Cmp(oldRevenue) != 0 {
		t.Error("profit not loaded correctly")
	}
}

// TestIntegrationValuePersistence verifies that changes made to the host persist between
// loads.
func TestIntegrationValuePersistence(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestIntegrationValuePersistence")
	if err != nil {
		t.Fatal(err)
	}

	// Change one of the features of the host persistence and save.
	ht.host.fileCounter += 1500
	oldFileCounter := ht.host.fileCounter
	err = ht.host.save()
	if err != nil {
		t.Fatal(err)
	}

	// Close the current host and create a new host pointing to the same file.
	ht.host.Close()
	newHost, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	// Check that the adjusted value has persisted.
	if newHost.fileCounter != oldFileCounter {
		t.Fatal(err)
	}
}

// TestUnitGetObligations checks that the getObligations method is correctly
// compiling contract obligations within the host.
func TestUnitGetObligations(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestUnitGetObligations")
	if err != nil {
		t.Fatal(err)
	}

	// Artificially fill the host with obligations to save.
	ob1 := &contractObligation{
		ID: types.FileContractID{1},
	}
	ob2 := &contractObligation{
		ID: types.FileContractID{2},
	}
	ht.host.obligationsByID[ob1.ID] = ob1
	ht.host.obligationsByID[ob2.ID] = ob2

	// Get the obligations from the host and check that it's a match.
	obligations := ht.host.getObligations()
	if len(obligations) != 2 {
		t.Fatal("getObligations did not fetch all of the obligations")
	}
	if obligations[0].ID == obligations[1].ID {
		t.Fatal("same obligation was grabbed twice")
	}
	if obligations[0].ID != ob1.ID && obligations[1].ID != ob1.ID {
		t.Fatal("ob1 not represented in fetched obligations")
	}
	if obligations[0].ID != ob2.ID && obligations[1].ID != ob2.ID {
		t.Fatal("ob2 not represented in fetched obligations")
	}
}

// TestUnitLoadObligations checks that a bunch of obligations can be correctly
// loaded into the host.
func TestUnitLoadObligations(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestUnitGetObligations")
	if err != nil {
		t.Fatal(err)
	}

	// Create a set of obligations to load into the host.
	ob1 := &contractObligation{
		ID: types.FileContractID{1},
		OriginTxn: types.Transaction{
			FileContracts: []types.FileContract{{
				WindowStart: 10e3,
			}},
		},
	}
	ob2 := &contractObligation{
		ID: types.FileContractID{2},
		OriginTxn: types.Transaction{
			FileContracts: []types.FileContract{{
				WindowStart: 20e3,
			}},
		},
	}
	ob3 := &contractObligation{
		ID: types.FileContractID{3},
		OriginTxn: types.Transaction{
			FileContracts: []types.FileContract{{
				WindowStart: 20e3,
			}},
		},
	}
	obs := []*contractObligation{ob1, ob2, ob3}
	ht.host.loadObligations(obs)

	// Check that the host has the obligations set up as expected.
	if ht.host.obligationsByID[ob1.ID].ID != ob1.ID {
		t.Error("ob1 not loaded correctly")
	}
	if ht.host.obligationsByID[ob2.ID].ID != ob2.ID {
		t.Error("ob2 not loaded correctly")
	}
	if ht.host.obligationsByID[ob3.ID].ID != ob3.ID {
		t.Error("ob3 not loaded correctly")
	}
}
