package renter

import (
	"bytes"
	"crypto/rand"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/types"
)

// a testHost simulates a host. It implements the hostdb.Uploader interface.
type testHost struct {
	ip   modules.NetAddress
	data []byte

	// used to simulate real-world conditions
	delay    time.Duration // transfers will take this long
	failRate int           // transfers will randomly fail with probability 1/failRate

	sync.Mutex
}

func (h *testHost) Address() modules.NetAddress  { return h.ip }
func (h *testHost) EndHeight() types.BlockHeight { return 0 }
func (h *testHost) Close() error                 { return nil }

func (h *testHost) ContractID() types.FileContractID {
	var fcid types.FileContractID
	copy(fcid[:], h.ip)
	return fcid
}

// Upload adds a piece to the testHost. It randomly fails according to the
// testHost's parameters.
func (h *testHost) Upload(data []byte) (offset uint64, err error) {
	// simulate I/O delay
	time.Sleep(h.delay)

	h.Lock()
	defer h.Unlock()

	// randomly fail
	if n, _ := crypto.RandIntn(h.failRate); n == 0 {
		return 0, errors.New("no data")
	}

	h.data = append(h.data, data...)
	return uint64(len(h.data) - len(data)), nil
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
	hosts := make([]hostdb.Uploader, rsc.NumPieces())
	for i := range hosts {
		hosts[i] = &testHost{
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
		if err != nil {
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
			encPiece := h.(*testHost).data[p.Offset : p.Offset+pieceSize+crypto.TwofishOverhead]
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
type offlineHostDB map[modules.NetAddress]bool

// ActiveHosts returns the set of hosts marked active in the offlineHostDB.
func (hdb offlineHostDB) ActiveHosts() (hosts []modules.HostSettings) {
	for addr, active := range hdb {
		if active {
			hosts = append(hosts, modules.HostSettings{IPAddress: addr})
		}
	}
	return
}

// AllHosts returns the entire contents of the offlineHostDB.
func (hdb offlineHostDB) AllHosts() (hosts []modules.HostSettings) {
	for addr := range hdb {
		hosts = append(hosts, modules.HostSettings{IPAddress: addr})
	}
	return
}

// AveragePrice is a stub implementation of the AveragePrice method.
func (hdb offlineHostDB) AveragePrice() types.Currency {
	return types.Currency{}
}

// NewPool is a stub implementation of the NewPool method.
func (hdb offlineHostDB) NewPool(uint64, types.BlockHeight) (hostdb.HostPool, error) {
	return nil, nil
}

// Renew is a stub implementation of the Renew method.
func (hdb offlineHostDB) Renew(types.FileContractID, types.BlockHeight) (types.FileContractID, error) {
	return types.FileContractID{}, nil
}

// TestOfflineChunks tests the offlineChunks method of the file type.
func TestOfflineChunks(t *testing.T) {
	// Create a mock hostdb.
	hdb := &offlineHostDB{
		"foo": false,
		"bar": false,
		"baz": true,
	}
	rsc, _ := NewRSCode(1, 1)
	f := &file{
		erasureCode: rsc,
		contracts: map[types.FileContractID]fileContract{
			{0}: {IP: "foo", Pieces: []pieceData{{0, 0, 0}, {1, 0, 0}}},
			{1}: {IP: "bar", Pieces: []pieceData{{0, 1, 0}}},
			{2}: {IP: "baz", Pieces: []pieceData{{1, 1, 0}}},
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
