package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

type testHost struct {
	data     []byte
	pieceMap map[uint64][]pieceData // key is chunkIndex
}

func (h *testHost) pieces(chunkIndex uint64) []pieceData {
	return h.pieceMap[chunkIndex]
}

func (h *testHost) fetch(p pieceData) ([]byte, error) {
	return h.data[p.Offset : p.Offset+p.Length], nil
}

// TestErasureDownload tests parallel downloading of erasure-coded data.
func TestErasureDownload(t *testing.T) {
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
	hosts := make([]fetcher, 3)
	for i := range hosts {
		hosts[i] = &testHost{
			pieceMap: make(map[uint64][]pieceData),
		}
	}

	// upload data to hosts
	const chunkSize = 100
	r := bytes.NewReader(data) // makes chunking easier
	chunk := make([]byte, chunkSize)
	var i uint64
	for i = uint64(0); ; i++ {
		_, err := io.ReadFull(r, chunk)
		if err == io.EOF {
			break
		} else if err != nil && err != io.ErrUnexpectedEOF {
			t.Fatal(err)
		}
		pieces, err := ecc.Encode(chunk)
		if err != nil {
			t.Fatal(err)
		}
		for j, p := range pieces {
			host := hosts[j%len(hosts)].(*testHost) // distribute evenly
			host.pieceMap[i] = append(host.pieceMap[i], pieceData{
				uint64(i),
				uint64(j),
				uint64(len(host.data)),
				uint64(len(p)),
			})
			host.data = append(host.data, p...)
		}
	}

	// check hosts (not strictly necessary)
	err = checkHosts(hosts, ecc.MinPieces(), i)
	if err != nil {
		t.Fatal(err)
	}

	// download data
	d := newDownload(ecc, chunkSize, dataSize, hosts, "", "")
	buf := new(bytes.Buffer)
	err = d.run(buf)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("recovered data does not match original")
	}
}
