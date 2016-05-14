package renter

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/modules/renter/contractor/proto"
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
	downloader contractor.Downloader
	pieceMap   map[uint64][]pieceData
	masterKey  crypto.TwofishKey
}

// pieces returns the pieces stored on this host that are part of a given
// chunk.
func (hf *hostFetcher) pieces(chunk uint64) []pieceData {
	return hf.pieceMap[chunk]
}

// fetch downloads the piece specified by p.
func (hf *hostFetcher) fetch(p pieceData) ([]byte, error) {
	// request piece
	data, err := hf.downloader.Sector(p.MerkleRoot)
	if err != nil {
		return nil, err
	}

	// generate decryption key
	key := deriveKey(hf.masterKey, p.Chunk, p.Piece)

	// decrypt and return
	return key.DecryptBytes(data)
}

// newHostFetcher creates a new hostFetcher.
func newHostFetcher(d contractor.Downloader, fc fileContract, masterKey crypto.TwofishKey) *hostFetcher {
	// make piece map
	pieceMap := make(map[uint64][]pieceData)
	for _, p := range fc.Pieces {
		pieceMap[p.Chunk] = append(pieceMap[p.Chunk], p)
	}
	return &hostFetcher{
		downloader: d,
		pieceMap:   pieceMap,
		masterKey:  masterKey,
	}
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
	// Lookup the file associated with the nickname.
	lockID := r.mu.Lock()
	file, exists := r.files[path]
	r.mu.Unlock(lockID)
	if !exists {
		return errors.New("no file with that path")
	}

	if !r.wallet.Unlocked() {
		return errors.New("wallet must be unlocked before downloading")
	}

	// Copy the file's metadata
	// TODO: this is ugly because we only have the Contracts method for
	// looking up contracts.
	contracts := make(map[*fileContract]proto.Contract)
	file.mu.RLock()
	for _, c := range r.hostContractor.Contracts() {
		fc, ok := file.contracts[c.ID]
		if ok {
			contracts[&fc] = c
		}
	}
	file.mu.RUnlock()
	r.log.Debugf("Starting Download, found %v contracts\n", len(contracts))

	if len(contracts) == 0 {
		return errors.New("no record of that file's contracts")
	}

	// Initiate connections to each host.
	var hosts []fetcher
	var errs []string
	for fc, c := range contracts {
		// TODO: connect in parallel
		d, err := r.hostContractor.Downloader(c)
		if err != nil {
			errs = append(errs, fmt.Sprintf("\t%v: %v", c.IP, err))
			continue
		}
		defer d.Close()
		hosts = append(hosts, newHostFetcher(d, *fc, file.masterKey))
	}
	if len(hosts) < file.erasureCode.MinPieces() {
		return errors.New("Could not connect to enough hosts:\n" + strings.Join(errs, "\n"))
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
