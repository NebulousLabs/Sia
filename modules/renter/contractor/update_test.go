package contractor

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

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
	editor, err := c.Editor(contract.ID, nil)
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
