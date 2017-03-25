package contractor

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// TestProcessConsensusUpdate tests that contracts are removed at the expected
// block height.
func TestProcessConsensusUpdate(t *testing.T) {
	// create contractor with a contract ending at height 20
	var stub newStub
	var rc modules.RenterContract
	rc.LastRevision.NewWindowStart = 20
	rc.FileContract.ValidProofOutputs = []types.SiacoinOutput{{}}
	c := &Contractor{
		cs:  stub,
		hdb: stub,
		contracts: map[types.FileContractID]modules.RenterContract{
			rc.ID: rc,
		},
		oldContracts: make(map[types.FileContractID]modules.RenterContract),
		persist:      new(memPersist),
		log:          persist.NewLogger(ioutil.Discard),
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
	t.Parallel()
	// create testing trio
	_, c, m, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// form a contract with the host
	a := modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(100), // 100 SC
		Hosts:       1,
		Period:      50,
		RenewWindow: 10,
	}
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	contract := c.Contracts()[0]

	// revise the contract
	editor, err := c.Editor(contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := fastrand.Bytes(int(modules.SectorSize))
	// insert the sector
	root, err := editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// mine until we enter the renew window
	renewHeight := contract.EndHeight() - c.allowance.RenewWindow
	for c.blockHeight < renewHeight {
		_, err := m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// wait for goroutine in ProcessConsensusChange to finish
	time.Sleep(100 * time.Millisecond)
	c.editLock.Lock()
	c.editLock.Unlock()

	// check renewed contract
	contract = c.Contracts()[0]
	if contract.FileContract.FileMerkleRoot != root {
		t.Fatal("wrong merkle root:", contract.FileContract.FileMerkleRoot)
	} else if contract.FileContract.FileSize != modules.SectorSize {
		t.Fatal("wrong file size:", contract.FileContract.FileSize)
	} else if contract.FileContract.RevisionNumber != 0 {
		t.Fatal("wrong revision number:", contract.FileContract.RevisionNumber)
	} else if contract.FileContract.WindowStart != c.blockHeight+c.allowance.Period {
		t.Fatal("wrong window start:", contract.FileContract.WindowStart)
	}
}

// TestIntegrationRenewInvalidate tests that editors and downloaders are
// properly invalidated when a renew is queued.
func TestIntegrationRenewInvalidate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	_, c, m, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// form a contract with the host
	a := modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(100), // 100 SC
		Hosts:       1,
		Period:      50,
		RenewWindow: 10,
	}
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	contract := c.Contracts()[0]

	// revise the contract
	editor, err := c.Editor(contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := fastrand.Bytes(int(modules.SectorSize))
	// insert the sector
	root, err := editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}

	// mine until we enter the renew window
	renewHeight := contract.EndHeight() - c.allowance.RenewWindow
	for c.blockHeight < renewHeight {
		_, err := m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// wait for goroutine in ProcessConsensusChange to finish
	time.Sleep(100 * time.Millisecond)
	c.editLock.Lock()
	c.editLock.Unlock()

	// check renewed contract
	contract = c.Contracts()[0]
	if contract.FileContract.FileMerkleRoot != root {
		t.Error("wrong merkle root:", contract.FileContract.FileMerkleRoot)
	} else if contract.FileContract.FileSize != modules.SectorSize {
		t.Error("wrong file size:", contract.FileContract.FileSize)
	} else if contract.FileContract.RevisionNumber != 0 {
		t.Error("wrong revision number:", contract.FileContract.RevisionNumber)
	} else if contract.FileContract.WindowStart != c.blockHeight+c.allowance.Period {
		t.Error("wrong window start:", contract.FileContract.WindowStart)
	}

	// editor should have been invalidated
	err = editor.Delete(crypto.Hash{})
	if err != errInvalidEditor {
		t.Error("expected invalid editor error; got", err)
	}
	editor.Close()

	// create a downloader
	downloader, err := c.Downloader(contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	// mine until we enter the renew window
	renewHeight = contract.EndHeight() - c.allowance.RenewWindow
	for c.blockHeight < renewHeight {
		_, err := m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// wait for goroutine in ProcessConsensusChange to finish
	time.Sleep(100 * time.Millisecond)
	c.editLock.Lock()
	c.editLock.Unlock()

	// downloader should have been invalidated
	_, err = downloader.Sector(crypto.Hash{})
	if err != errInvalidDownloader {
		t.Error("expected invalid downloader error; got", err)
	}
	downloader.Close()
}
