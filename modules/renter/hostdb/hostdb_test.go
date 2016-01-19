package hostdb

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// bareHostDB returns a HostDB with its fields initialized, but without any
// dependencies or scanning threads. It is only intended for use in unit tests.
func bareHostDB() *HostDB {
	return &HostDB{
		contracts:   make(map[types.FileContractID]hostContract),
		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),
	}
}

// newStub is used to test the New function. It implements all of the hostdb's
// dependencies.
type newStub struct{}

// consensus set stubs
func (newStub) ConsensusSetSubscribe(modules.ConsensusSetSubscriber) {}

// wallet stubs
func (newStub) NextAddress() (uc types.UnlockConditions, err error) { return }
func (newStub) StartTransaction() modules.TransactionBuilder        { return nil }

// transaction pool stubs
func (newStub) AcceptTransactionSet([]types.Transaction) error { return nil }

// TestNew tests the New function.
func TestNew(t *testing.T) {
	// Using a stub implementation of the dependencies is fine, as long as its
	// non-nil.
	var stub newStub
	dir := build.TempDir("hostdb", "TestNew")

	// Sane values.
	_, err := New(stub, stub, stub, dir)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Nil consensus set.
	_, err = New(nil, stub, stub, dir)
	if err != errNilCS {
		t.Fatalf("expected %v, got %v", errNilCS, err)
	}

	// Nil wallet.
	_, err = New(stub, nil, stub, dir)
	if err != errNilWallet {
		t.Fatalf("expected %v, got %v", errNilWallet, err)
	}

	// Nil transaction pool.
	_, err = New(stub, stub, nil, dir)
	if err != errNilTpool {
		t.Fatalf("expected %v, got %v", errNilTpool, err)
	}

	// Bad persistDir.
	_, err = New(stub, stub, stub, "")
	if err == nil {
		t.Fatal("expected invalid directory, got nil")
	}

	// Corrupted persist file.
	ioutil.WriteFile(filepath.Join(dir, "hostdb.json"), []byte{1, 2, 3}, 0666)
	_, err = New(stub, stub, stub, dir)
	if err == nil {
		t.Fatalf("expected invalid json, got nil")
	}

	// Corrupted logfile.
	os.RemoveAll(filepath.Join(dir, "hostdb.log"))
	f, err := os.OpenFile(filepath.Join(dir, "hostdb.log"), os.O_CREATE, 0000)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, err = New(stub, stub, stub, dir)
	if err == nil {
		t.Fatal("expected permissions error, got nil")
	}
}
