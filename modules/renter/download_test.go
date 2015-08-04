package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

type downloader struct {
	ecc       modules.ECC
	chunkSize int
	remaining int
	hosts     []fileHost
	reqChans  []chan int
	respChans []chan []byte
}

func newDownloader(ecc modules.ECC, chunkSize, fileSize int, hosts []fileHost) *downloader {
	// create channels
	reqChans := make([]chan int, len(hosts))
	for i := range reqChans {
		reqChans[i] = make(chan int)
	}
	respChans := make([]chan []byte, ecc.NumPieces())
	for i := range respChans {
		respChans[i] = make(chan []byte)
	}

	return &downloader{
		ecc:       ecc,
		chunkSize: chunkSize,
		remaining: fileSize,
		hosts:     hosts,
		reqChans:  reqChans,
		respChans: respChans,
	}
}

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

func (d *downloader) worker(host fileHost, reqChan chan int) {
	for chunkIndex := range reqChan {
		for _, p := range host.pieces(chunkIndex) {
			data, err := host.fetch(p)
			if err != nil {
				data = nil
			}
			d.respChans[p.piece] <- data
		}
	}
}

func (d *downloader) run(w io.Writer) error {
	// spawn download workers
	for i, h := range d.hosts {
		go d.worker(h, d.reqChans[i])
	}

	defer func() {
		// close request channels, terminating the worker goroutines
		for _, ch := range d.reqChans {
			close(ch)
		}
	}()

	chunk := make([][]byte, d.ecc.NumPieces())
	for i := 0; d.remaining > 0; i++ {
		// tell all workers to download chunk i
		for _, ch := range d.reqChans {
			ch <- i
		}
		// load pieces into chunk
		for j, ch := range d.respChans {
			chunk[j] = <-ch
		}

		// Write pieces to w. We always write chunkSize bytes unless this is
		// the last chunk; in that case, we write the remainder.
		n := d.chunkSize
		if n > d.remaining {
			n = d.remaining
		}
		err := d.ecc.Recover(chunk, uint64(n), w)
		if err != nil {
			return err
		}
		d.remaining -= d.chunkSize
	}

	return nil
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
	hosts := make([]testHost, 3)
	for i := range hosts {
		hosts[i].pieceMap = make(map[int][]pieceData)
	}

	// upload data to hosts
	const chunkSize = 100
	r := bytes.NewReader(data) // makes chunking easier
	chunk := make([]byte, chunkSize)
	for i := 0; ; i++ {
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
			host := &hosts[j%len(hosts)] // distribute evenly
			host.pieceMap[i] = append(host.pieceMap[i], pieceData{j, len(host.data), len(p)})
			host.data = append(host.data, p...)
		}
	}

	// annoying -- have to convert to proper interface
	var hs []fileHost
	for i := range hosts {
		hs = append(hs, &hosts[i])
	}

	// download data
	d := newDownloader(ecc, chunkSize, dataSize, hs)
	buf := new(bytes.Buffer)
	err = d.run(buf)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("recovered data does not match original")
	}
}
