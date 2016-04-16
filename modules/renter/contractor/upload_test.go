package contractor

import (
	"github.com/NebulousLabs/Sia/crypto"
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
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

// editorDialer is used to test the Editor method.
type editorDialer func() (net.Conn, error)

func (dial editorDialer) DialTimeout(addr modules.NetAddress, timeout time.Duration) (net.Conn, error) {
	return dial()
}

// TestEditor tests the Editor method.
func TestEditor(t *testing.T) {
	// use a mock hostdb to supply hosts
	hdb := &editorHostDB{
		hosts: make(map[modules.NetAddress]modules.HostDBEntry),
	}
	c := &Contractor{
		hdb: hdb,
	}

	// empty contract
	_, err := c.Editor(Contract{})
	if err == nil {
		t.Error("expected error, got nil")
	}

	// expired contract
	c.blockHeight = 3
	_, err = c.Editor(Contract{})
	if err == nil {
		t.Error("expected error, got nil")
	}
	c.blockHeight = 0

	// expensive host
	hostSecretKey, hostPublicKey := crypto.GenerateKeyPairDeterministic([32]byte{})
	dbe := modules.HostDBEntry{
		PublicKey: types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       hostPublicKey[:],
		},
	}
	dbe.AcceptingContracts = true
	dbe.StoragePrice = types.NewCurrency64(^uint64(0))
	hdb.hosts["foo"] = dbe
	_, err = c.Editor(Contract{IP: "foo"})
	if err == nil {
		t.Error("expected error, got nil")
	}

	// invalid contract
	dbe.StoragePrice = types.NewCurrency64(500)
	hdb.hosts["bar"] = dbe
	_, err = c.Editor(Contract{IP: "bar"})
	if err == nil {
		t.Error("expected error, got nil")
	}

	// spent contract
	contract := Contract{
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

	// give contract more value; it should be valid now
	contract.LastRevision.NewValidProofOutputs[0].Value = types.NewCurrency64(modules.SectorSize * 500)

	// contract with unresponsive host
	c.dialer = editorDialer(func() (net.Conn, error) {
		return nil, net.ErrWriteToConnected
	})
	_, err = c.Editor(contract)
	if err != net.ErrWriteToConnected {
		t.Error("expected ErrWriteToConnected, got", err)
	}

	// contract with a disconnecting host
	c.dialer = editorDialer(func() (net.Conn, error) {
		ourPipe, theirPipe := net.Pipe()
		ourPipe.Close()
		return theirPipe, nil
	})
	_, err = c.Editor(contract)
	if err == nil {
		t.Errorf("expected err, got nil")
	}

	// contract with a disconnecting host
	c.dialer = editorDialer(func() (net.Conn, error) {
		// create an in-memory conn and spawn a goroutine to handle our half
		ourConn, theirConn := net.Pipe()
		go func() {
			// read the RPC and immediately close
			encoding.ReadObject(ourConn, new(types.Specifier), types.SpecifierLen)
			ourConn.Close()
		}()
		return theirConn, nil
	})
	_, err = c.Editor(contract)
	if err == nil {
		t.Error("expected err, got nil")
	}

	// contract with a valid host
	c.dialer = editorDialer(func() (net.Conn, error) {
		// create an in-memory conn and spawn a goroutine to handle our half
		ourConn, theirConn := net.Pipe()
		go func() {
			// read specifier
			encoding.ReadObject(ourConn, new(types.Specifier), types.SpecifierLen)
			// send settings
			crypto.WriteSignedObject(ourConn, dbe.HostExternalSettings, hostSecretKey)
			// read acceptance
			encoding.ReadObject(ourConn, new(string), modules.MaxErrorSize)
			// read contract ID
			encoding.ReadObject(ourConn, new(types.FileContractID), 32)
			// send transaction
			encoding.WriteObject(ourConn, contract.LastRevisionTxn)
			ourConn.Close()
		}()
		return theirConn, nil
	})
	_, err = c.Editor(contract)
	if err != nil {
		t.Error(err)
	}
}
