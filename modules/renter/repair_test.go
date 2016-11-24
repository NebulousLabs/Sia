package renter

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/types"
)

// hostErr and hostErrs are helpers for reporting repair errors. The actual
// Error implementations aren't that important; we just need to be able to
// extract the NetAddress of the failed host.

type hostErr struct {
	host modules.NetAddress
	err  error
}

func (he hostErr) Error() string {
	return fmt.Sprintf("host %v failed: %v", he.host, he.err)
}

type hostErrs []*hostErr

func (hs hostErrs) Error() string {
	var errs []error
	for _, h := range hs {
		errs = append(errs, h)
	}
	return build.JoinErrors(errs, "\n").Error()
}

// offlineChunks returns the chunks belonging to "offline" hosts -- hosts that
// do not meet uptime requirements. Importantly, only chunks missing more than
// half their redundancy are returned.
func (f *file) offlineChunks(hdb hostDB) map[uint64][]uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// mark all pieces belonging to offline hosts.
	offline := make(map[uint64][]uint64)
	for _, fc := range f.contracts {
		if hdb.IsOffline(fc.IP) {
			for _, p := range fc.Pieces {
				offline[p.Chunk] = append(offline[p.Chunk], p.Piece)
			}
		}
	}
	// filter out chunks missing less than half of their redundancy
	filtered := make(map[uint64][]uint64)
	for chunk, pieces := range offline {
		if len(pieces) > f.erasureCode.NumPieces()/2 {
			filtered[chunk] = pieces
		}
	}
	return filtered
}

// repair attempts to repair a file chunk by uploading its pieces to more
// hosts.
func (f *file) repair(chunkIndex uint64, missingPieces []uint64, r io.ReaderAt, hosts []contractor.Editor) error {
	// read chunk data and encode
	chunk := make([]byte, f.chunkSize())
	_, err := r.ReadAt(chunk, int64(chunkIndex*f.chunkSize()))
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return err
	}
	pieces, err := f.erasureCode.Encode(chunk)
	if err != nil {
		return err
	}
	// encrypt pieces
	for i := range pieces {
		key := deriveKey(f.masterKey, chunkIndex, uint64(i))
		pieces[i], err = key.EncryptBytes(pieces[i])
		if err != nil {
			return err
		}
	}

	// upload one piece per host
	numPieces := len(missingPieces)
	if len(hosts) < numPieces {
		numPieces = len(hosts)
	}
	errChan := make(chan *hostErr)
	for i := 0; i < numPieces; i++ {
		go func(pieceIndex uint64, host contractor.Editor) {
			// upload data to host
			root, err := host.Upload(pieces[pieceIndex])
			if err != nil {
				errChan <- &hostErr{host.Address(), err}
				return
			}

			// create contract entry, if necessary
			f.mu.Lock()
			contract, ok := f.contracts[host.ContractID()]
			if !ok {
				contract = fileContract{
					ID:          host.ContractID(),
					IP:          host.Address(),
					WindowStart: host.EndHeight(),
				}
			}

			// update contract
			contract.Pieces = append(contract.Pieces, pieceData{
				Chunk:      chunkIndex,
				Piece:      pieceIndex,
				MerkleRoot: root,
			})
			f.contracts[host.ContractID()] = contract
			f.mu.Unlock()
			errChan <- nil
		}(missingPieces[i], hosts[i])
	}
	var errs hostErrs
	for i := 0; i < numPieces; i++ {
		err := <-errChan
		if err != nil {
			errs = append(errs, err)
		}
	}
	if errs != nil {
		return errs
	}

	return nil
}

// incompleteChunks returns a set of chunks on a file that are not at full
// redundancy. incompleteChunks will only return chunks/peices if there are
// hosts available to accept the data.
func (f *file) incompleteChunks() map[uint64][]uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	present := make([][]bool, f.numChunks())
	for i := range present {
		present[i] = make([]bool, f.erasureCode.NumPieces())
	}
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			present[p.Chunk][p.Piece] = true
		}
	}

	incomplete := make(map[uint64][]uint64)
	for chunkIndex, pieceBools := range present {
		for pieceIndex, ok := range pieceBools {
			if !ok {
				incomplete[uint64(chunkIndex)] = append(incomplete[uint64(chunkIndex)], uint64(pieceIndex))
			}
		}
	}
	return incomplete
}

// a testHost simulates a host. It implements the contractor.Editor interface.
type testHost struct {
	ip      modules.NetAddress
	sectors map[crypto.Hash][]byte

	// used to simulate real-world conditions
	delay    time.Duration // transfers will take this long
	failRate int           // transfers will randomly fail with probability 1/failRate

	sync.Mutex
}

// stub implementations of the contractor.Editor methods
func (h *testHost) Address() modules.NetAddress                           { return h.ip }
func (h *testHost) Delete(crypto.Hash) error                              { return nil }
func (h *testHost) Modify(crypto.Hash, crypto.Hash, uint64, []byte) error { return nil }
func (h *testHost) EndHeight() types.BlockHeight                          { return 0 }
func (h *testHost) Close() error                                          { return nil }

// ContractID returns a fake (but unique) file contract ID.
func (h *testHost) ContractID() types.FileContractID {
	var fcid types.FileContractID
	copy(fcid[:], h.ip)
	return fcid
}

// Upload adds a piece to the testHost. It randomly fails according to the
// testHost's parameters.
func (h *testHost) Upload(data []byte) (crypto.Hash, error) {
	// simulate I/O delay
	time.Sleep(h.delay)

	h.Lock()
	defer h.Unlock()

	// randomly fail
	if n, _ := crypto.RandIntn(h.failRate); n == 0 {
		return crypto.Hash{}, errors.New("no data")
	}

	root := crypto.MerkleRoot(data)
	h.sectors[root] = data
	return root, nil
}

// TestRepair tests the repair method of the file type.
func TestRepair(t *testing.T) {
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
	hosts := make([]contractor.Editor, rsc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
			sectors:  make(map[crypto.Hash][]byte),
			ip:       modules.NetAddress(strconv.Itoa(i)),
			delay:    time.Duration(i) * time.Millisecond,
			failRate: 5, // 20% failure rate
		}
	}
	// make one host really slow
	hosts[0].(*testHost).delay = 100 * time.Millisecond
	// make one host always fail
	hosts[1].(*testHost).failRate = 1

	// upload data to hosts
	f := newFile("foo", rsc, pieceSize, dataSize)
	r := bytes.NewReader(data)
	for chunk, pieces := range f.incompleteChunks() {
		err = f.repair(chunk, pieces, r, hosts)
		// hostErrs are non-fatal
		if _, ok := err.(hostErrs); ok {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
	}

	// download data
	chunks := make([][][]byte, f.numChunks())
	for i := uint64(0); i < f.numChunks(); i++ {
		chunks[i] = make([][]byte, rsc.NumPieces())
	}
	for _, h := range hosts {
		contract, exists := f.contracts[h.ContractID()]
		if !exists {
			continue
		}
		for _, p := range contract.Pieces {
			encPiece := h.(*testHost).sectors[p.MerkleRoot]
			piece, err := deriveKey(f.masterKey, p.Chunk, p.Piece).DecryptBytes(encPiece)
			if err != nil {
				t.Fatal(err)
			}
			chunks[p.Chunk][p.Piece] = piece
		}
	}
	buf := new(bytes.Buffer)
	for _, chunk := range chunks {
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
		// These metrics can be used to assess the efficiency of the repair
		// algorithm.

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

// offlineHostDB is a mocked hostDB, used for testing the offlineChunks method
// of the file type. It is implemented as a map from NetAddresses to booleans,
// where the bool indicates whether the host is active.
type offlineHostDB struct {
	stubHostDB
	hosts map[modules.NetAddress]bool
}

// IsOffline is a stub implementation of the IsOffline method.
func (hdb *offlineHostDB) IsOffline(addr modules.NetAddress) bool {
	return !hdb.hosts[addr]
}

// TestOfflineChunks tests the offlineChunks method of the file type.
func TestOfflineChunks(t *testing.T) {
	// Create a mock hostdb.
	hdb := &offlineHostDB{
		hosts: map[modules.NetAddress]bool{
			"foo": false,
			"bar": false,
			"baz": true,
		},
	}
	rsc, _ := NewRSCode(1, 1)
	f := &file{
		erasureCode: rsc,
		contracts: map[types.FileContractID]fileContract{
			{0}: {IP: "foo", Pieces: []pieceData{{0, 0, crypto.Hash{}}, {1, 0, crypto.Hash{}}}},
			{1}: {IP: "bar", Pieces: []pieceData{{0, 1, crypto.Hash{}}}},
			{2}: {IP: "baz", Pieces: []pieceData{{1, 1, crypto.Hash{}}}},
		},
	}

	// pieces 0.0, 0.1, and 1.0 are offline. Since redundancy is 1,
	// offlineChunks should report only chunk 0 as needing repair.
	expChunks := map[uint64][]uint64{
		0: {0, 1},
	}
	chunks := f.offlineChunks(hdb)
	if !reflect.DeepEqual(chunks, expChunks) {
		// pieces may have been in a different order
		if !reflect.DeepEqual(chunks, map[uint64][]uint64{0: {1, 0}}) {
			t.Fatalf("offlineChunks did not return correct chunks: expected %v, got %v", expChunks, chunks)
		}
	}
}
