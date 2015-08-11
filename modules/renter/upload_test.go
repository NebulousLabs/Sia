package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// disallow arbitrary offset+length: require indexes
// you can only download a full index at a time
// nice benefits for merkle trees
// but then you need a separate merkle root...

type uploader interface {
	// connect initiates the connection to the uploader, creating a
	// fileContract (or returning a pre-existing one).
	connect() (fileContract, error)

	// upload
	upload(uint64, uint64, []byte) error
}

func (h *testHost) connect() (fileContract, error) {
	return fileContract{}, nil
}

func (h *testHost) upload(chunkIndex, pieceIndex uint64, piece []byte) error {
	h.pieceMap[chunkIndex] = append(h.pieceMap[chunkIndex], pieceData{
		chunkIndex,
		pieceIndex,
		uint64(len(h.data)),
		uint64(len(piece)),
	})
	h.data = append(h.data, piece...)
	return nil
}

func newFile(ecc modules.ECC, pieceSize, fileSize uint64) *dfile {
	key, _ := crypto.GenerateTwofishKey()
	return &dfile{
		Size:      fileSize,
		MasterKey: key,
		ecc:       ecc,
		pieceSize: pieceSize,
	}
}

func (f *dfile) upload(r io.Reader, hosts []uploader) error {
	for _, h := range hosts {
		h.connect()
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
		// upload pieces to hosts
		for j, h := range hosts {
			err := h.upload(i, uint64(j), pieces[j])
			if err != nil {
				return err
			}
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
