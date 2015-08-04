package renter

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

type pieceData struct {
	piece  int
	offset int
	length int
}

type fileHost interface {
	pieces(chunkIndex int) []pieceData
	fetch(pieceData) ([]byte, error)
}

type testHost struct {
	data     []byte
	pieceMap map[int][]pieceData // key is chunkIndex
}

func (h testHost) pieces(chunkIndex int) []pieceData {
	return h.pieceMap[chunkIndex]
}

func (h testHost) fetch(p pieceData) ([]byte, error) {
	return h.data[p.offset : p.offset+p.length], nil
}

func retrieve(host fileHost, reqChan chan int, respChans []chan []byte) {
	for chunkIndex := range reqChan {
		for _, p := range host.pieces(chunkIndex) {
			data, err := host.fetch(p)
			if err != nil {
				data = nil
			}
			respChans[p.piece] <- data
		}
	}
}

func orchestrate(ecc modules.ECC, dataSize int, chunkSize int, reqChans []chan int, respChans []chan []byte) ([]byte, error) {
	defer func() {
		// close request channels, terminating the worker goroutines
		for _, ch := range reqChans {
			close(ch)
		}
	}()

	buf := new(bytes.Buffer) // recovered data will be written here
	chunk := make([][]byte, len(respChans))
	bytesLeft := dataSize
	for i := 0; bytesLeft > 0; i++ {
		// tell all workers to download chunk i
		for _, ch := range reqChans {
			ch <- i
		}
		// load pieces into chunk
		for j, ch := range respChans {
			chunk[j] = <-ch
		}

		// write pieces to buf
		err := ecc.Recover(chunk, uint64(chunkSize), buf)
		if err != nil {
			return nil, err
		}
		bytesLeft -= chunkSize
	}

	buf.Truncate(int(dataSize))

	return buf.Bytes(), nil
}

// TestErasureDownload tests parallel downloading of erasure-coded data.
func TestErasureDownload(t *testing.T) {
	// generate data
	const dataSize = 900
	data := make([]byte, dataSize)
	rand.Read(data)

	// create RS encoder
	ecc, err := NewRSCode(2, 10)
	if err != nil {
		t.Fatal(err)
	}

	// create hosts
	hosts := make([]testHost, 3)
	for i := range hosts {
		hosts[i].pieceMap = make(map[int][]pieceData)
	}

	// upload data to hosts
	const chunkSize = 100
	for i := 0; i < len(data)/chunkSize; i++ {
		pieces, err := ecc.Encode(data[i*100 : (i+1)*100])
		if err != nil {
			t.Fatal(err)
		}
		for j, p := range pieces {
			host := &hosts[j%len(hosts)] // distribute evenly
			host.pieceMap[i] = append(host.pieceMap[i], pieceData{j, len(host.data), len(p)})
			host.data = append(host.data, p...)
		}
	}

	// create communication channels
	reqChans := make([]chan int, len(hosts))
	for i := range reqChans {
		reqChans[i] = make(chan int)
	}
	respChans := make([]chan []byte, ecc.NumPieces())
	for i := range respChans {
		respChans[i] = make(chan []byte)
	}

	// spawn download workers
	for i, h := range hosts {
		go retrieve(h, reqChans[i], respChans)
	}

	// download data
	rec, err := orchestrate(ecc, dataSize, chunkSize, reqChans, respChans)
	if err != nil {
		t.Fatal(err)
	}

	if len(rec) < len(data) || !bytes.Equal(rec[:len(data)], data) {
		t.Fatal("recovered data does not match original")
	}
}
