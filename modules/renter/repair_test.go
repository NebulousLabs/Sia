package renter

import (
	"bytes"
	"crypto/rand"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
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
	err = f.upload(bytes.NewReader(data), hosts)
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
		t.Fatal("recovereed data does not match original")
	}
}
