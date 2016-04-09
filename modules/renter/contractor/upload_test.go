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
	hosts map[modules.NetAddress]modules.HostExternalSettings
}

func (hdb editorHostDB) Host(addr modules.NetAddress) (modules.HostExternalSettings, bool) {
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
		hosts: make(map[modules.NetAddress]modules.HostExternalSettings),
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
	hdb.hosts["foo"] = modules.HostExternalSettings{ContractPrice: types.NewCurrency64(^uint64(0))}
	_, err = c.Editor(Contract{IP: "foo"})
	if err == nil {
		t.Error("expected error, got nil")
	}

	// invalid contract
	hdb.hosts["bar"] = modules.HostExternalSettings{ContractPrice: types.NewCurrency64(500)}
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
	contract.LastRevision.NewValidProofOutputs[0].Value = types.NewCurrency64(SectorSize * 500)

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
			encoding.ReadObject(ourConn, new(types.Specifier), types.SpecifierLen)
			encoding.ReadObject(ourConn, new(types.FileContractID), crypto.HashSize)
			ourConn.Close()
		}()
		return theirConn, nil
	})
	_, err = c.Editor(contract)
	if err != nil {
		t.Error(err)
	}
}
