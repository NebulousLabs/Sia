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

// deriveKey derives the key used to encrypt and decrypt a specific file piece.
func deriveKey(masterKey crypto.TwofishKey, chunkIndex, pieceIndex uint64) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, chunkIndex, pieceIndex))
}

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
	err := encoding.WriteObject(hf.conn, modules.DownloadRequest{p.Offset, hf.pieceSize})
	if err != nil {
		return nil, err
	}
	// TODO: would it be more efficient to do this manually?
	// i.e. read directly into a bytes.Buffer
	var b []byte
	err = encoding.ReadObject(hf.conn, &b, hf.pieceSize)
	if err != nil {
		return nil, err
	}

	// generate decryption key
	key := deriveKey(hf.masterKey, p.Chunk, p.Piece)

	// decrypt and return
	return key.DecryptBytes(b)
}

func (hf *hostFetcher) Close() error {
	// ignore error; we'll need to close conn anyway
	encoding.WriteObject(hf.conn, modules.DownloadRequest{0, 0})
	return hf.conn.Close()
}

func newHostFetcher(fc fileContract, masterKey crypto.TwofishKey) (*hostFetcher, error) {
	conn, err := net.DialTimeout("tcp", string(fc.IP), 5*time.Second)
	if err != nil {
		return nil, err
	}

	// send RPC
	err = encoding.WriteObject(conn, modules.RPCDownload)
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
			return errors.New("insufficient hosts to recover file")
		}
	}
	return nil
}

// A download is a file download that has been queued by the renter. It
// implements the modules.DownloadInfo interface.
type download struct {
	// NOTE: received is the first field to ensure 64-bit alignment, which is
	// required for atomic operations.
	received uint64

	startTime   time.Time
	nickname    string
	destination string

	ecc       modules.ECC
	chunkSize uint64
	fileSize  uint64
	hosts     []fetcher
}

// StartTime is when the download was initiated.
func (d *download) StartTime() time.Time {
	return d.startTime
}

// Filesize is the size of the file being downloaded.
func (d *download) Filesize() uint64 {
	return d.fileSize
}

// Received is the number of bytes downloaded so far.
func (d *download) Received() uint64 {
	return atomic.LoadUint64(&d.received)
}

// Destination is the filepath that the file was downloaded into.
func (d *download) Destination() string {
	return d.destination
}

// Nickname is the identifier assigned to the file when it was uploaded.
func (d *download) Nickname() string {
	return d.nickname
}

// downloadWorker fetches pieces from a host as directed by reqChan. It sends
// the fetched pieces down the appropriate respChan.
func downloadWorker(host fetcher, reqChan chan uint64, respChans []chan []byte) {
	for chunkIndex := range reqChan {
		for _, p := range host.pieces(chunkIndex) {
			data, err := host.fetch(p)
			if err != nil {
				data = nil
			}
			respChans[p.Piece] <- data
		}
	}
}

// run performs the actual download. It spawns one worker per host, and
// instructs them to sequentially download chunks. It then writes the
// recovered chunks to w.
func (d *download) run(w io.Writer) error {
	// create request and response channels
	reqChans := make([]chan uint64, len(d.hosts))
	for i := range reqChans {
		reqChans[i] = make(chan uint64)
	}
	respChans := make([]chan []byte, d.ecc.NumPieces())
	for i := range respChans {
		respChans[i] = make(chan []byte)
	}

	// spawn download workers
	for i, h := range d.hosts {
		go downloadWorker(h, reqChans[i], respChans)
		defer close(reqChans[i])
	}

	var received uint64
	for i := uint64(0); received < d.fileSize; i++ {
		// tell all workers to download chunk i
		for _, ch := range reqChans {
			ch <- i
		}
		// load pieces into chunk
		// TODO: this deadlocks if any pieces are missing.
		chunk := make([][]byte, d.ecc.NumPieces())
		for j, ch := range respChans {
			chunk[j] = <-ch
		}

		// Write pieces to w. We always write chunkSize bytes unless this is
		// the last chunk; in that case, we write the remainder.
		n := d.chunkSize
		if n > d.fileSize-received {
			n = d.fileSize - received
		}
		err := d.ecc.Recover(chunk, uint64(n), w)
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
		ecc:       f.ecc,
		chunkSize: f.chunkSize(),
		fileSize:  f.Size,
		hosts:     hosts,

		startTime:   time.Now(),
		received:    0,
		nickname:    f.Name,
		destination: destination,
	}
}

// Download downloads a file, identified by its nickname, to the destination
// specified.
func (r *Renter) Download(nickname, destination string) error {
	// Lookup the file associated with the nickname.
	lockID := r.mu.Lock()
	file, exists := r.files[nickname]
	r.mu.Unlock(lockID)
	if !exists {
		return errors.New("no file of that nickname")
	}

	// Create file on disk.
	f, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer f.Close() // should be okay even if file is Remove'd

	// Initiate connections to each host.
	// TODO: Ideally, we only need 'file.ecc.MinPieces' hosts. Otherwise we
	// wind up connecting and then disconnecting without transferring any
	// data, which is wasteful of host resources.
	var hosts []fetcher
	for _, fc := range file.Contracts {
		// TODO: connect in parallel
		hf, err := newHostFetcher(fc, file.MasterKey)
		if err != nil {
			continue
		}
		defer hf.Close()
		hosts = append(hosts, hf)
	}

	// Check that this host set is sufficient to download the file.
	err = checkHosts(hosts, file.ecc.MinPieces(), file.numChunks())
	if err != nil {
		return err
	}

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
		downloads[i] = r.downloadQueue[len(r.downloadQueue)-i-1]
	}
	return downloads
}
