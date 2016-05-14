package contractor

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor/proto"
	"github.com/NebulousLabs/Sia/types"
)

// editorHostDB is used to test the Editor method.
type editorHostDB struct {
	stubHostDB
	hosts map[modules.NetAddress]modules.HostDBEntry
}

func (hdb editorHostDB) Host(addr modules.NetAddress) (modules.HostDBEntry, bool) {
	h, ok := hdb.hosts[addr]
	return h, ok
}

// TestEditor tests the failure conditions of the Editor method. The method is
// more fully tested in the host integration test.
func TestEditor(t *testing.T) {
	// use a mock hostdb to supply hosts
	hdb := &editorHostDB{
		hosts: make(map[modules.NetAddress]modules.HostDBEntry),
	}
	c := &Contractor{
		hdb: hdb,
	}

	// empty contract
	_, err := c.Editor(proto.Contract{})
	if err == nil {
		t.Error("expected error, got nil")
	}

	// expired contract
	c.blockHeight = 3
	_, err = c.Editor(proto.Contract{})
	if err == nil {
		t.Error("expected error, got nil")
	}
	c.blockHeight = 0

	// expensive host
	_, hostPublicKey := crypto.GenerateKeyPairDeterministic([32]byte{})
	dbe := modules.HostDBEntry{
		PublicKey: types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       hostPublicKey[:],
		},
	}
	dbe.AcceptingContracts = true
	dbe.StoragePrice = types.NewCurrency64(^uint64(0))
	hdb.hosts["foo"] = dbe
	_, err = c.Editor(proto.Contract{IP: "foo"})
	if err == nil {
		t.Error("expected error, got nil")
	}

	// invalid contract
	dbe.StoragePrice = types.NewCurrency64(500)
	hdb.hosts["bar"] = dbe
	_, err = c.Editor(proto.Contract{IP: "bar"})
	if err == nil {
		t.Error("expected error, got nil")
	}

	// spent contract
	contract := proto.Contract{
		IP: "bar",
		LastRevision: types.FileContractRevision{
			NewValidProofOutputs: []types.SiacoinOutput{
				{Value: types.NewCurrency64(0)},
				{Value: types.NewCurrency64(^uint64(0))},
			},
		},
	}
	_, err = c.Editor(contract)
	if err == nil {
		t.Error("expected error, got nil")
	}
}
