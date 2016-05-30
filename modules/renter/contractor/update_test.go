package contractor

import (
	"io/ioutil"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// TestProcessConsensusUpdate tests that contracts are removed at the expected
// block height.
func TestProcessConsensusUpdate(t *testing.T) {
	// create contractor with a contract ending at height 20
	var rc modules.RenterContract
	rc.LastRevision.NewWindowStart = 20
	rc.FileContract.ValidProofOutputs = []types.SiacoinOutput{{}}
	c := &Contractor{
		contracts: map[types.FileContractID]modules.RenterContract{
			rc.ID: rc,
		},
		persist: new(memPersist),
		log:     persist.NewLogger(ioutil.Discard),
	}

	// process 20 blocks; contract should remain
	cc := modules.ConsensusChange{
		// just need to increment blockheight by 1
		AppliedBlocks: []types.Block{{}},
	}
	for i := 0; i < 20; i++ {
		c.ProcessConsensusChange(cc)
	}
	if len(c.contracts) != 1 {
		t.Error("expected 1 contract, got", len(c.contracts))
	}

	// process one more block; contract should be removed
	c.ProcessConsensusChange(cc)
	if len(c.contracts) != 0 {
		t.Error("expected 0 contracts, got", len(c.contracts))
	}
}

// TestIntegrationAutoRenew tests that contracts are automatically renwed at
// the expected block height.
func TestIntegrationAutoRenew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create testing trio
	h, c, m, err := newTestingTrio("TestIntegrationAutoRenew")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, modules.SectorSize*10, c.blockHeight+50)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
	// insert the sector
	root, err := editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// set allowance (manually) so that Contractor will auto-renew
	c.mu.Lock()
	c.allowance = modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(100), // 100 SC
		Hosts:       1,
		Period:      50,
		RenewWindow: 10,
	}
	c.mu.Unlock()

	// mine until we enter the renew window
	renewHeight := contract.EndHeight() - c.allowance.RenewWindow
	for c.blockHeight < renewHeight {
		_, err := m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// check renewed contract
	contract = c.Contracts()[0]
	if contract.FileContract.FileMerkleRoot != root {
		t.Fatal(contract.FileContract.FileMerkleRoot)
	} else if contract.FileContract.FileSize != modules.SectorSize {
		t.Fatal(contract.FileContract.FileSize)
	} else if contract.FileContract.RevisionNumber != 0 {
		t.Fatal(contract.FileContract.RevisionNumber)
	} else if contract.FileContract.WindowStart != c.blockHeight+c.allowance.Period {
		t.Fatal(contract.FileContract.WindowStart)
	}
}
