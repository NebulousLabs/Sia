package renter

import (
	"bytes"
	"crypto/rand"
	"reflect"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
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
	rsc, err := NewRSCode(4, 6)
	if err != nil {
		t.Fatal(err)
	}

	// create hosts
	const pieceSize = 10
	hosts := make([]uploader, rsc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			ip:        modules.NetAddress(strconv.Itoa(i)),
			pieceMap:  make(map[uint64][]pieceData),
			pieceSize: pieceSize,
			failRate:  3, // 33% failure rate
		}
	}
	// make one host always fail
	hosts[0].(*testHost).failRate = 1

	// upload data to hosts
	f := newFile("foo", rsc, pieceSize, dataSize)
	err = f.repair(bytes.NewReader(data), f.incompleteChunks(), hosts)
	if err != nil {
		t.Fatal(err)
	}

	// download data (should fail)
	dhosts := make([]fetcher, len(hosts))
	for i := range dhosts {
		dhosts[i] = hosts[i].(*testHost)
	}
	d := f.newDownload(dhosts, "")
	buf := new(bytes.Buffer)
	err = d.run(buf)
	if err != errInsufficientPieces {
		t.Fatalf("download should not have succeeded: expected %v, got %v", errInsufficientPieces, err)
	}

	// repair until file becomes available
	const maxAttempts = 20
	for i := 0; i < maxAttempts; i++ {
		err = f.repair(bytes.NewReader(data), f.incompleteChunks(), hosts)
		if err != nil {
			t.Fatal(err)
		}

		buf.Reset()
		err = d.run(buf)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("file not repaired after %v attempts: %v", maxAttempts, err)
	}

	// check data integrity
	buf.Truncate(dataSize)
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("recovered data does not match original")
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
