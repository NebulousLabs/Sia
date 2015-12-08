package renter

import (
	"bytes"
	"crypto/rand"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
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
