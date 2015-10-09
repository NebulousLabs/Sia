package renter

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

func (h *testHost) addPiece(p uploadPiece) error {
	// simulate I/O delay
	time.Sleep(h.delay)

	h.Lock()
	defer h.Unlock()

	// randomly fail
	if n, _ := crypto.RandIntn(h.failRate); n == 0 {
		return crypto.ErrNilInput
	}

	h.pieceMap[p.chunkIndex] = append(h.pieceMap[p.chunkIndex], pieceData{
		p.chunkIndex,
		p.pieceIndex,
		uint64(len(h.data)),
	})
	h.data = append(h.data, p.data...)
	return nil
}

func (h *testHost) fileContract() fileContract {
	var fc fileContract
	for _, ps := range h.pieceMap {
		fc.Pieces = append(fc.Pieces, ps...)
	}
	fc.IP = h.ip
	return fc
}

func (h *testHost) addr() modules.NetAddress {
	return h.ip
}

// TestErasureUpload tests parallel uploading of erasure-coded data.
func TestErasureUpload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// generate data
	const dataSize = 777
	data := make([]byte, dataSize)
	rand.Read(data)

	// create Reed-Solomon encoder
	rsc, err := NewRSCode(2, 10)
	if err != nil {
		t.Fatal(err)
	}

	// create hosts
	const pieceSize = 10
	hosts := make([]uploader, rsc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			pieceMap:  make(map[uint64][]pieceData),
			pieceSize: pieceSize,
			delay:     time.Duration(i) * time.Millisecond,
			failRate:  5, // 20% failure rate
		}
	}
	// make one host really slow
	hosts[0].(*testHost).delay = 100 * time.Millisecond
	// make one host always fail
	hosts[1].(*testHost).failRate = 1

	// upload data to hosts
	f := newFile("foo", rsc, pieceSize, dataSize)
	err = f.upload(bytes.NewReader(data), hosts)
	if err != nil {
		t.Fatal(err)
	}

	// download data
	buf := new(bytes.Buffer)
	for i := uint64(0); i < f.numChunks(); i++ {
		chunk := make([][]byte, rsc.NumPieces())
		for _, h := range hosts {
			host := h.(*testHost)
			for _, p := range host.pieceMap[i] {
				chunk[p.Piece] = host.data[p.Offset : p.Offset+pieceSize]
			}
		}
		err = rsc.Recover(chunk, f.chunkSize(), buf)
		if err != nil {
			t.Fatal(err)
		}
	}
	buf.Truncate(dataSize)

	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("recovered data does not match original")
	}

	/*
		for i, h := range hosts {
			host := h.(*testHost)
			pieces := 0
			for _, p := range host.pieceMap {
				pieces += len(p)
			}
			t.Logf("Host #: %d\tDelay: %v\t# Pieces: %v\t# Chunks: %d", i, host.delay, pieces, len(host.pieceMap))
		}
	*/
}
