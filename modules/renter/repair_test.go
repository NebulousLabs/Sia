package renter

import (
	"bytes"
	"crypto/rand"
	"reflect"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/types"
)

// TestRepair tests that the repair method can repeatedly improve the
// redundancy of an unavailable file until it becomes available.
func TestRepair(t *testing.T) {
	// generate data
	const dataSize = 777
	data := make([]byte, dataSize)
	rand.Read(data)

	// create Reed-Solomon encoder
	rsc, err := NewRSCode(8, 2)
	if err != nil {
		t.Fatal(err)
	}

	// create hosts
	const pieceSize = 10
	hosts := make([]hostdb.Uploader, rsc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			ip:       modules.NetAddress(strconv.Itoa(i)),
			failRate: 2, // 50% failure rate
		}
	}
	// make one host always fail
	hosts[0].(*testHost).failRate = 1

	// upload data to hosts
	f := newFile("foo", rsc, pieceSize, dataSize)
	r := bytes.NewReader(data)
	for chunk, pieces := range f.incompleteChunks() {
		err = f.repair(chunk, pieces, r, hosts)
		if err != nil {
			t.Fatal(err)
		}
	}

	// file should not be available after first pass
	if f.available() {
		t.Fatalf("file should not be available: %v%%", f.uploadProgress())
	}

	// repair until file becomes available
	const maxAttempts = 20
	for i := 0; i < maxAttempts; i++ {
		for chunk, pieces := range f.incompleteChunks() {
			err = f.repair(chunk, pieces, r, hosts)
			if err != nil {
				t.Fatal(err)
			}
		}
		if f.available() {
			break
		}
	}
	if !f.available() {
		t.Fatalf("file not repaired to availability after %v attempts: %v", maxAttempts, err)
	}
}

// offlineHostDB is a mocked hostDB, used for testing the offlineChunks method
// of the file type. It is implemented as a map from NetAddresses to booleans,
// where the bool indicates whether the host is active.
type offlineHostDB map[modules.NetAddress]bool

// ActiveHosts returns the set of hosts marked active in the offlineHostDB.
func (hdb offlineHostDB) ActiveHosts() (hosts []modules.HostSettings) {
	for addr, active := range hdb {
		if active {
			hosts = append(hosts, modules.HostSettings{IPAddress: addr})
		}
	}
	return
}

// AllHosts returns the entire contents of the offlineHostDB.
func (hdb offlineHostDB) AllHosts() (hosts []modules.HostSettings) {
	for addr := range hdb {
		hosts = append(hosts, modules.HostSettings{IPAddress: addr})
	}
	return
}

// AveragePrice is a stub implementation of the AveragePrice method.
func (hdb offlineHostDB) AveragePrice() types.Currency {
	return types.Currency{}
}

// NewPool is a stub implementation of the NewPool method.
func (hdb offlineHostDB) NewPool(uint64, types.BlockHeight) (hostdb.HostPool, error) {
	return nil, nil
}

// Renew is a stub implementation of the Renew method.
func (hdb offlineHostDB) Renew(types.FileContractID, types.BlockHeight) (types.FileContractID, error) {
	return types.FileContractID{}, nil
}

// TestOfflineChunks tests the offlineChunks method of the file type.
func TestOfflineChunks(t *testing.T) {
	// Create a mock hostdb.
	hdb := &offlineHostDB{
		"foo": false,
		"bar": false,
		"baz": true,
	}
	rsc, _ := NewRSCode(1, 1)
	f := &file{
		erasureCode: rsc,
		contracts: map[types.FileContractID]fileContract{
			{0}: {IP: "foo", Pieces: []pieceData{{0, 0, 0}, {1, 0, 0}}},
			{1}: {IP: "bar", Pieces: []pieceData{{0, 1, 0}}},
			{2}: {IP: "baz", Pieces: []pieceData{{1, 1, 0}}},
		},
	}

	// pieces 0.0, 0.1, and 1.0 are offline. Since redundancy is 1,
	// offlineChunks should report only chunk 0 as needing repair.
	expChunks := map[uint64][]uint64{
		0: {0, 1},
	}
	chunks := f.offlineChunks(hdb)
	if !reflect.DeepEqual(chunks, expChunks) {
		// pieces may have been in a different order
		if !reflect.DeepEqual(chunks, map[uint64][]uint64{0: {1, 0}}) {
			t.Fatalf("offlineChunks did not return correct chunks: expected %v, got %v", expChunks, chunks)
		}
	}
}
