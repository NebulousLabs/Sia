package renter

import (
	"errors"
	"io"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	errInsufficientHosts  = errors.New("insufficient hosts to recover file")
	errInsufficientPieces = errors.New("couldn't fetch enough pieces to recover data")
)

// A fetcher fetches pieces from a host. This interface exists to facilitate
// easy testing.
type fetcher interface {
	// pieces returns the set of pieces corresponding to a given chunk.
	pieces(chunk uint64) []pieceData

	// fetch returns the data specified by piece metadata.
	fetch(pieceData) ([]byte, error)
}

// A hostFetcher fetches pieces from a host. It implements the fetcher
// interface.
type hostFetcher struct {
	conn      net.Conn
	pieceMap  map[uint64][]pieceData
	pieceSize uint64
	masterKey crypto.TwofishKey
}

// pieces returns the pieces stored on this host that are part of a given
// chunk.
func (hf *hostFetcher) pieces(chunk uint64) []pieceData {
	return hf.pieceMap[chunk]
}

// fetch downloads the piece specified by p.
func (hf *hostFetcher) fetch(p pieceData) ([]byte, error) {
	hf.conn.SetDeadline(time.Now().Add(2 * time.Minute)) // sufficient to transfer 4 MB over 250 kbps
	defer hf.conn.SetDeadline(time.Time{})
	// request piece
	err := encoding.WriteObject(hf.conn, errors.New("TODO: switch to new download protocol"))
	if err != nil {
		return nil, err
	}

	// download piece
	data := make([]byte, hf.pieceSize)
	_, err = io.ReadFull(hf.conn, data)
	if err != nil {
		return nil, err
	}

	// generate decryption key
	key := deriveKey(hf.masterKey, p.Chunk, p.Piece)

	// decrypt and return
	return key.DecryptBytes(data)
}

func (hf *hostFetcher) Close() error {
	// ignore error; we'll need to close conn anyway
	encoding.WriteObject(hf.conn, errors.New("TODO: switch to new download protocol"))
	return hf.conn.Close()
}

// newHostFetcher creates a new hostFetcher by connecting to a host.
// TODO: We may not wind up requesting data from this, which means we will
// connect and then disconnect without making any actual requests (but holding
// the connection open the entire time). This is wasteful of host resources.
// Consider only opening the connection after the first request has been made.
func newHostFetcher(fc fileContract, pieceSize uint64, masterKey crypto.TwofishKey) (*hostFetcher, error) {
	conn, err := net.DialTimeout("tcp", string(fc.IP), 15*time.Second)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	defer conn.SetDeadline(time.Time{})

	// send RPC
	err = encoding.WriteObject(conn, modules.RPCDownload)
	if err != nil {
		return nil, err
	}

	// send contract ID
	err = encoding.WriteObject(conn, fc.ID)
	if err != nil {
		return nil, err
	}

	// make piece map
	pieceMap := make(map[uint64][]pieceData)
	for _, p := range fc.Pieces {
		pieceMap[p.Chunk] = append(pieceMap[p.Chunk], p)
	}
	return &hostFetcher{
		conn:      conn,
		pieceMap:  pieceMap,
		pieceSize: pieceSize + crypto.TwofishOverhead,
		masterKey: masterKey,
	}, nil
}

// checkHosts checks that a set of hosts is sufficient to download a file.
func checkHosts(hosts []fetcher, minPieces int, numChunks uint64) error {
	for i := uint64(0); i < numChunks; i++ {
		pieces := 0
		for _, h := range hosts {
			pieces += len(h.pieces(i))
		}
		if pieces < minPieces {
			return errInsufficientHosts
		}
	}
	return nil
}

// A download is a file download that has been queued by the renter.
type download struct {
	// NOTE: received is the first field to ensure 64-bit alignment, which is
	// required for atomic operations.
	received uint64

	startTime   time.Time
	siapath     string
	destination string

	erasureCode modules.ErasureCoder
	chunkSize   uint64
	fileSize    uint64
	hosts       []fetcher
}

// getPiece locates and downloads a specific piece.
func (d *download) getPiece(chunkIndex, pieceIndex uint64) []byte {
	for _, h := range d.hosts {
		for _, p := range h.pieces(chunkIndex) {
			if p.Piece == pieceIndex {
				data, err := h.fetch(p)
				if err != nil {
					break // try next host
				}
				return data
			}
		}
	}
	return nil
}

// run performs the actual download. It spawns one worker per host, and
// instructs them to sequentially download chunks. It then writes the
// recovered chunks to w.
func (d *download) run(w io.Writer) error {
	var received uint64
	for i := uint64(0); received < d.fileSize; i++ {
		// load pieces into chunk
		chunk := make([][]byte, d.erasureCode.NumPieces())
		left := d.erasureCode.MinPieces()
		// pick hosts at random
		chunkOrder, err := crypto.Perm(len(chunk))
		if err != nil {
			return err
		}
		for _, j := range chunkOrder {
			chunk[j] = d.getPiece(i, uint64(j))
			if chunk[j] != nil {
				left--
			} else {
			}
			if left == 0 {
				break
			}
		}
		if left != 0 {
			return errInsufficientPieces
		}

		// Write pieces to w. We always write chunkSize bytes unless this is
		// the last chunk; in that case, we write the remainder.
		n := d.chunkSize
		if n > d.fileSize-received {
			n = d.fileSize - received
		}
		err = d.erasureCode.Recover(chunk, uint64(n), w)
		if err != nil {
			return err
		}
		received += n
		atomic.AddUint64(&d.received, n)
	}

	return nil
}

// newDownload initializes and returns a download object.
func (f *file) newDownload(hosts []fetcher, destination string) *download {
	return &download{
		erasureCode: f.erasureCode,
		chunkSize:   f.chunkSize(),
		fileSize:    f.size,
		hosts:       hosts,

		startTime:   time.Now(),
		received:    0,
		siapath:     f.name,
		destination: destination,
	}
}

// Download downloads a file, identified by its path, to the destination
// specified.
func (r *Renter) Download(path, destination string) error {
	if true { // evade go vet
		return errors.New("TODO: fix download code")
	}

	// Lookup the file associated with the nickname.
	lockID := r.mu.Lock()
	file, exists := r.files[path]
	r.mu.Unlock(lockID)
	if !exists {
		return errors.New("no file with that path")
	}

	// Copy the file's metadata
	var contracts []fileContract
	file.mu.RLock()
	for _, fc := range file.contracts {
		contracts = append(contracts, fc)
	}
	file.mu.RUnlock()

	// Initiate connections to each host.
	var hosts []fetcher
	for _, fc := range contracts {
		// TODO: connect in parallel
		hf, err := newHostFetcher(fc, file.pieceSize, file.masterKey)
		if err != nil {
			continue
		}
		defer hf.Close()
		hosts = append(hosts, hf)
	}

	// Check that this host set is sufficient to download the file.
	err := checkHosts(hosts, file.erasureCode.MinPieces(), file.numChunks())
	if err != nil {
		return err
	}

	// Create file on disk with the correct permissions.
	perm := os.FileMode(file.mode)
	if perm == 0 {
		// sane default
		perm = 0666
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_RDWR|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()

	// Create the download object.
	d := file.newDownload(hosts, destination)

	// Add the download to the download queue.
	lockID = r.mu.Lock()
	r.downloadQueue = append(r.downloadQueue, d)
	r.mu.Unlock(lockID)

	// Perform download.
	err = d.run(f)
	if err != nil {
		// File could not be downloaded; delete the copy on disk.
		os.Remove(destination)
		return err
	}

	return nil
}

// DownloadQueue returns the list of downloads in the queue.
func (r *Renter) DownloadQueue() []modules.DownloadInfo {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// order from most recent to least recent
	downloads := make([]modules.DownloadInfo, len(r.downloadQueue))
	for i := range r.downloadQueue {
		d := r.downloadQueue[len(r.downloadQueue)-i-1]
		downloads[i] = modules.DownloadInfo{
			SiaPath:     d.siapath,
			Destination: d.destination,
			Filesize:    d.fileSize,
			Received:    atomic.LoadUint64(&d.received),
			StartTime:   d.startTime,
		}
	}
	return downloads
}
