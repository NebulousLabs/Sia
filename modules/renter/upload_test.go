package renter

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"
)

func (h *testHost) addPiece(p uploadPiece) (*fileContract, error) {
	h.pieceMap[p.chunkIndex] = append(h.pieceMap[p.chunkIndex], pieceData{
		p.chunkIndex,
		p.pieceIndex,
		uint64(len(h.data)),
		uint64(len(p.data)),
	})
	h.data = append(h.data, p.data...)
	// simulate I/O delay
	time.Sleep(h.delay)
	return nil, nil
}

// TestErasureUpload tests parallel uploading of erasure-coded data.
func TestErasureUpload(t *testing.T) {
	// generate data
	const dataSize = 777
	data := make([]byte, dataSize)
	rand.Read(data)

	// create RS encoder
	ecc, err := NewRSCode(2, 10)
	if err != nil {
		t.Fatal(err)
	}

	// create hosts
	hosts := make([]uploader, ecc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			pieceMap: make(map[uint64][]pieceData),
			delay:    1 * time.Millisecond,
		}
	}
	// make one host really slow
	// ideally, the test should take exactly this long.
	hosts[0].(*testHost).delay = 100 * time.Millisecond

	// upload data to hosts
	const pieceSize = 10
	f := newFile(ecc, pieceSize, dataSize)
	r := bytes.NewReader(data)
	err = f.upload(r, hosts)
	if err != nil {
		t.Fatal(err)
	}

	// download data
	buf := new(bytes.Buffer)
	chunk := make([][]byte, ecc.NumPieces())
	for i := uint64(0); i < f.numChunks(); i++ {
		for _, h := range hosts {
			host := h.(*testHost)
			for _, p := range host.pieceMap[i] {
				chunk[p.Piece] = host.data[p.Offset : p.Offset+p.Length]
			}
		}
		err = ecc.Recover(chunk, f.chunkSize(), buf)
		if err != nil {
			t.Fatal(err)
		}
	}
	buf.Truncate(dataSize)

	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("recovered data does not match original")
	}
}
