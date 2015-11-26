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

// mockHostDB mocks the functions of the HostDB.
// TODO: might be unnecessary if we implement a per-chunk repair fn
type mockHostDB struct {
	hosts []*testHost
}

func (hdb *mockHostDB) ActiveHosts() []modules.HostSettings { return nil }
func (hdb *mockHostDB) AllHosts() []modules.HostSettings    { return nil }
func (hdb *mockHostDB) AveragePrice() types.Currency        { return types.NewCurrency64(0) }

func (hdb *mockHostDB) UniqueHosts(n int, old []hostdb.Uploader) (ups []hostdb.Uploader) {
	for i := 0; i < n && i < len(hdb.hosts); i++ {
		ups = append(ups, hdb.hosts[i])
	}
	return
}

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
	hosts := make([]*testHost, rsc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			ip:       modules.NetAddress(strconv.Itoa(i)),
			failRate: 2, // 50% failure rate
		}
	}
	// make one host always fail
	hosts[0].failRate = 1

	// create hostdb
	hdb := &mockHostDB{hosts: hosts}

	// upload data to hosts
	f := newFile("foo", rsc, pieceSize, dataSize)
	err = f.repair(bytes.NewReader(data), f.incompleteChunks(), hdb)
	if err != nil {
		t.Fatal(err)
	}

	// file should not be available after first pass
	if f.available() {
		t.Fatalf("file should not be available: %v%%", f.uploadProgress())
	}

	// repair until file becomes available
	const maxAttempts = 20
	for i := 0; i < maxAttempts; i++ {
		err = f.repair(bytes.NewReader(data), f.incompleteChunks(), hdb)
		if err != nil {
			t.Fatal(err)
		}
		if f.available() {
			break
		}
	}
	if !f.available() {
		t.Fatalf("file not repaired to availability after %v attempts: %v", maxAttempts, err)
	}
}

// TestSubtractRepairMap tests the subtract method of the repairMap type.
func TestSubtractRepairMap(t *testing.T) {
	orig := repairMap{
		10: {1, 2, 3},
		12: {4, 5, 6},
	}
	sub := repairMap{
		10: {1, 2},
		12: {5},
		13: {7, 8, 9},
	}
	exp := repairMap{
		10: {3},
		12: {4, 6},
	}
	res := orig.subtract(sub)
	if !reflect.DeepEqual(exp, res) {
		t.Fatal("maps were merged incorrectly:", exp, res)
	}
}

// TestChunksBelow tests the chunksBelow method of the file type.
func TestChunksBelow(t *testing.T) {
	var f1, f2, f3, f4 file
	f1.contracts = map[types.FileContractID]fileContract{
		{}: {WindowStart: 1, Pieces: []pieceData{{}}},
	}
	f2.contracts = map[types.FileContractID]fileContract{
		{}: {WindowStart: 1, Pieces: []pieceData{{}}},
	}
	f3.contracts = map[types.FileContractID]fileContract{
		{}: {WindowStart: 1, Pieces: []pieceData{{}}},
	}
	f4.contracts = map[types.FileContractID]fileContract{
		{0}: {WindowStart: 1, Pieces: []pieceData{{}}},
		{1}: {WindowStart: 2, Pieces: []pieceData{{}}},
	}
	tests := []struct {
		f         file
		endHeight types.BlockHeight
		expChunks int
	}{
		{f1, 0, 0},
		{f2, 1, 0},
		{f3, 2, 1},
		{f4, 2, 0},
	}
	for i, test := range tests[3:] {
		if n := len(test.f.chunksBelow(test.endHeight)); n != test.expChunks {
			t.Errorf("%d: expected %v expiring chunks, got %v", i, test.expChunks, n)
		}
	}
}
