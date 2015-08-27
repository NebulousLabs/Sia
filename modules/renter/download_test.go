package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
)

type testHost struct {
	data      []byte
	pieceMap  map[uint64][]pieceData // key is chunkIndex
	pieceSize uint64
	nAttempt  int // total number of download attempts
	nFetch    int // number of successfull download attempts

	// used to simulate real-world conditions
	delay    time.Duration // download will take this long
	failRate int           // download will randomly fail with probability 1/failRate
}

func (h *testHost) pieces(chunkIndex uint64) []pieceData {
	return h.pieceMap[chunkIndex]
}

func (h *testHost) fetch(p pieceData) ([]byte, error) {
	h.nAttempt++
	time.Sleep(h.delay)
	// randomly fail
	if n, _ := crypto.RandIntn(h.failRate); n == 0 {
		return nil, io.EOF
	}
	h.nFetch++
	return h.data[p.Offset : p.Offset+h.pieceSize], nil
}

// TestErasureDownload tests parallel downloading of erasure-coded data.
func TestErasureDownload(t *testing.T) {
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
	hosts := make([]fetcher, rsc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			pieceMap:  make(map[uint64][]pieceData),
			pieceSize: pieceSize,

			delay:    time.Millisecond,
			failRate: 5, // 20% failure rate
		}
	}
	// make one host really slow
	hosts[0].(*testHost).delay = 100 * time.Millisecond
	// make one host always fail
	hosts[1].(*testHost).failRate = 1

	// upload data to hosts
	r := bytes.NewReader(data) // makes chunking easier
	chunk := make([]byte, pieceSize*rsc.MinPieces())
	var i uint64
	for i = uint64(0); ; i++ {
		_, err := io.ReadFull(r, chunk)
		if err == io.EOF {
			break
		} else if err != nil && err != io.ErrUnexpectedEOF {
			t.Fatal(err)
		}
		pieces, err := rsc.Encode(chunk)
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
	err = checkHosts(hosts, rsc.MinPieces(), i)
	if err != nil {
		t.Fatal(err)
	}

	// download data
	d := newFile("foo", rsc, pieceSize, dataSize).newDownload(hosts, "")
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
			h := h.(*testHost)
			t.Logf("Host %2d:  Fetched: %v/%v", i, h.nFetch, h.nAttempt)
			totFetch += h.nAttempt

		}
		t.Log("Optimal fetches:", i*uint64(rsc.MinPieces()))
		t.Log("Total fetches:  ", totFetch)
	*/
}
