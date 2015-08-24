package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
	"time"
)

type testHost struct {
	data      []byte
	pieceMap  map[uint64][]pieceData // key is chunkIndex
	pieceSize uint64
	nFetch    int

	delay time.Duration // used to simulate real-world conditions
}

func (h *testHost) pieces(chunkIndex uint64) []pieceData {
	return h.pieceMap[chunkIndex]
}

func (h *testHost) fetch(p pieceData) ([]byte, error) {
	time.Sleep(h.delay)
	h.nFetch++
	return h.data[p.Offset : p.Offset+h.pieceSize], nil
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
	const pieceSize = 10
	hosts := make([]fetcher, ecc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			pieceMap:  make(map[uint64][]pieceData),
			delay:     time.Millisecond,
			pieceSize: pieceSize,
		}
	}
	// make one host really slow
	hosts[0].(*testHost).delay = 10 * time.Millisecond

	// upload data to hosts
	r := bytes.NewReader(data) // makes chunking easier
	chunk := make([]byte, pieceSize*ecc.MinPieces())
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
	d := newFile("foo", ecc, pieceSize, dataSize).newDownload(hosts, "")
	buf := new(bytes.Buffer)
	err = d.run(buf)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("recovered data does not match original")
	}

	/*
		totFetch := 0
		for i, h := range hosts {
			t.Logf("Host #: %d  \tFetched: %v", i, h.(*testHost).nFetch)
			totFetch += h.(*testHost).nFetch

		}
		t.Log("Optimal fetches:", i*uint64(ecc.MinPieces()))
		t.Log("Total fetches:  ", totFetch)
	*/
}
