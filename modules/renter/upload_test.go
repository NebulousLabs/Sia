package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

type uploadPiece struct {
	data       []byte
	chunkIndex uint64
	pieceIndex uint64
}

type uploader interface {
	// connect initiates the connection to the uploader.
	connect() (fileContract, error)

	// upload uploads a piece to the uploader.
	upload(uploadPiece) (fileContract, error)
}

func (h *testHost) connect() (fileContract, error) {
	return fileContract{}, nil
}

func (h *testHost) upload(p uploadPiece) (fileContract, error) {
	h.pieceMap[p.chunkIndex] = append(h.pieceMap[p.chunkIndex], pieceData{
		p.chunkIndex,
		p.pieceIndex,
		uint64(len(h.data)),
		uint64(len(p.data)),
	})
	h.data = append(h.data, p.data...)
	return fileContract{}, nil
}

func newFile(ecc modules.ECC, pieceSize, fileSize uint64) *dfile {
	key, _ := crypto.GenerateTwofishKey()
	return &dfile{
		Size:      fileSize,
		Contracts: make(map[modules.NetAddress]fileContract),
		MasterKey: key,
		ecc:       ecc,
		pieceSize: pieceSize,
	}
}

func (f *dfile) worker(host uploader, reqChan chan uploadPiece, respChan chan *fileContract) {
	// TODO: move connect outside worker
	_, err := host.connect()
	if err != nil {
		respChan <- nil
		return
	}
	for req := range reqChan {
		contract, err := host.upload(req)
		if err != nil {
			respChan <- nil
			return // this host is now dead to us; upload will use a new one
		}
		respChan <- &contract
	}
}

func (f *dfile) upload(r io.Reader, hosts []uploader) error {
	// create request/response channels and spawn workers
	reqChans := make([]chan uploadPiece, len(hosts))
	respChans := make([]chan *fileContract, len(hosts))
	for i, h := range hosts {
		reqChans[i] = make(chan uploadPiece)
		respChans[i] = make(chan *fileContract)
		go f.worker(h, reqChans[i], respChans[i])
	}

	chunk := make([]byte, f.chunkSize())
	for i := uint64(0); ; i++ {
		// read next chunk
		n, err := io.ReadFull(r, chunk)
		if err == io.EOF {
			break
		} else if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
		// encode
		pieces, err := f.ecc.Encode(chunk)
		if err != nil {
			return err
		}
		// send upload requests to workers
		for j, ch := range reqChans {
			ch <- uploadPiece{pieces[j], i, uint64(j)}
		}
		// read upload responses from workers
		for _, ch := range respChans {
			contract := <-ch
			if contract == nil {
				// choose new host somehow
				//go f.worker(newhost, reqChans[j], respChans[j])
				continue
			}
			f.Contracts[contract.IP] = *contract
		}
		f.uploaded += uint64(n)
	}
	return nil
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
		}
	}

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
			host := h.(fetcher)
			for _, p := range host.pieces(i) {
				chunk[p.Piece], err = host.fetch(p)
				if err != nil {
					t.Fatal(err)
				}
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
