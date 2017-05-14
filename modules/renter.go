package modules

import (
	"encoding/json"
	"io"
	"time"

	"net/http"
	"os"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

const (
	defaultFilePerm = 0666
	// RenterDir is the name of the directory that is used to store the
	// renter's persistent data.
	RenterDir = "renter"
)

// An ErasureCoder is an error-correcting encoder and decoder.
type ErasureCoder interface {
	// NumPieces is the number of pieces returned by Encode.
	NumPieces() int

	// MinPieces is the minimum number of pieces that must be present to
	// recover the original data.
	MinPieces() int

	// Encode splits data into equal-length pieces, with some pieces
	// containing parity data.
	Encode(data []byte) ([][]byte, error)

	// Recover recovers the original data from pieces (including parity) and
	// writes it to w. pieces should be identical to the slice returned by
	// Encode (length and order must be preserved), but with missing elements
	// set to nil. n is the number of bytes to be written to w; this is
	// necessary because pieces may have been padded with zeros during
	// encoding.
	Recover(pieces [][]byte, n uint64, w io.Writer) error
}

// An Allowance dictates how much the Renter is allowed to spend in a given
// period. Note that funds are spent on both storage and bandwidth.
type Allowance struct {
	Funds       types.Currency    `json:"funds"`
	Hosts       uint64            `json:"hosts"`
	Period      types.BlockHeight `json:"period"`
	RenewWindow types.BlockHeight `json:"renewwindow"`
}

// DownloadInfo provides information about a file that has been requested for
// download.
type DownloadInfo struct {
	SiaPath     string         `json:"siapath"`
	Destination DownloadWriter `json:"destination"`
	Filesize    uint64         `json:"filesize"`
	Received    uint64         `json:"received"`
	StartTime   time.Time      `json:"starttime"`
	Error       string         `json:"error"`
}

// DownloadWriter provides an interface which all output writers have to implement.
type DownloadWriter interface {
	WriteAt(b []byte, off int64) (int, error)
	String() string
}

// DownloadFileWriter is a file-backed implementation of DownloadWriter.
type DownloadFileWriter struct {
	f        *os.File
	Location string
	offset   uint64
}

// NewDownloadFileWriter creates a new instance of a DownloadWriter backed by the file named.
func NewDownloadFileWriter(fname string, offset, length uint64) *DownloadFileWriter {
	l, _ := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	return &DownloadFileWriter{
		f:        l,
		Location: fname,
		offset:   offset,
	}
}

// WriteAt writes the passed bytes at the specified offset.
func (dw *DownloadFileWriter) WriteAt(b []byte, off int64) (int, error) {
	fileOffset := off - int64(dw.offset)

	r, err := dw.f.WriteAt(b, fileOffset)
	if err != nil {
		build.ExtendErr("unable to write to download destination", err)
	}
	dw.f.Sync()

	return r, err
}

// String returns the destination of the DownloadFileWriter as a string.
func (dw *DownloadFileWriter) String() string {
	return dw.Location
}

// DownloadHttpWriter is a http response writer-backed implementation of DownloadWriter.
// The writer writes all content that is written to the current `offset` directly to the ResponseWriter,
// and buffers all content that is written at other offsets.
// After every write to the ResponseWriter the `offset` and `length` fields are updated, and buffer content written until
type DownloadHttpWriter struct {
	w              http.ResponseWriter
	offset         int            // The index in the original file of the last byte written to the response writer.
	firstByteIndex int            // The index of the first byte in the original file.
	length         int            // The total size of the slice to be written.
	buffer         map[int][]byte // Buffer used for storing the chunks until download finished.
}

// NewDownloadHttpWriter creates a new instance of http.ResponseWriter backed DownloadWriter.
func NewDownloadHttpWriter(w http.ResponseWriter, offset, length uint64) *DownloadHttpWriter {
	return &DownloadHttpWriter{
		w:              w,
		offset:         0,           // Current offset in the output file.
		firstByteIndex: int(offset), // Index of first byte in original file.
		length:         int(length),
		buffer:         make(map[int][]byte),
	}
}

// WriteAt buffers parts of the file until the entire file can be
// flushed to the client. Returns the number of bytes written or an error.
func (dw *DownloadHttpWriter) WriteAt(b []byte, off int64) (int, error) {
	// Write bytes to buffer.
	offsetInBuffer := int(off) - dw.firstByteIndex
	dw.buffer[offsetInBuffer] = b

	// Send all chunks to the client that can be sent.
	var totalDataSend = 0
	for {
		data, exists := dw.buffer[dw.offset]
		if exists {
			// Send data to client.
			dw.w.Write(data)

			// Remove chunk from map.
			delete(dw.buffer, dw.offset)

			// Increment offset to point to the beginning of the next chunk.
			dw.offset += len(data)
			totalDataSend += len(data)
		} else {
			break
		}
	}

	return totalDataSend, nil
}

// String returns the destination of the DownloadHttpWriter as a string.
func (dw *DownloadHttpWriter) String() string {
	return "httpresp"
}

// FileUploadParams contains the information used by the Renter to upload a
// file.
type FileUploadParams struct {
	Source      string
	SiaPath     string
	ErasureCode ErasureCoder
}

// FileInfo provides information about a file.
type FileInfo struct {
	SiaPath        string            `json:"siapath"`
	Filesize       uint64            `json:"filesize"`
	Available      bool              `json:"available"`
	Renewing       bool              `json:"renewing"`
	Redundancy     float64           `json:"redundancy"`
	UploadProgress float64           `json:"uploadprogress"`
	Expiration     types.BlockHeight `json:"expiration"`
}

// A HostDBEntry represents one host entry in the Renter's host DB. It
// aggregates the host's external settings and metrics with its public key.
type HostDBEntry struct {
	HostExternalSettings

	// FirstSeen is the last block height at which this host was announced.
	FirstSeen types.BlockHeight `json:"firstseen"`

	// Measurements that have been taken on the host. The most recent
	// measurements are kept in full detail, historic ones are compressed into
	// the historic values.
	HistoricDowntime time.Duration `json:"historicdowntime"`
	HistoricUptime   time.Duration `json:"historicuptime"`
	ScanHistory      HostDBScans   `json:"scanhistory"`

	// The public key of the host, stored separately to minimize risk of certain
	// MitM based vulnerabilities.
	PublicKey types.SiaPublicKey `json:"publickey"`
}

// HostDBScan represents a single scan event.
type HostDBScan struct {
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
}

// HostScoreBreakdown provides a piece-by-piece explanation of why a host has
// the score that they do.
//
// NOTE: Renters are free to use whatever scoring they feel appropriate for
// hosts. Some renters will outright blacklist or whitelist sets of hosts. The
// results provided by this struct can only be used as a guide, and may vary
// significantly from machine to machine.
type HostScoreBreakdown struct {
	Score types.Currency `json:"score"`

	AgeAdjustment              float64 `json:"ageadjustment"`
	BurnAdjustment             float64 `json:"burnadjustment"`
	CollateralAdjustment       float64 `json:"collateraladjustment"`
	PriceAdjustment            float64 `json:"pricesmultiplier"`
	StorageRemainingAdjustment float64 `json:"storageremainingadjustment"`
	UptimeAdjustment           float64 `json:"uptimeadjustment"`
	VersionAdjustment          float64 `json:"versionadjustment"`
}

// RenterPriceEstimation contains a bunch of files estimating the costs of
// various operations on the network.
type RenterPriceEstimation struct {
	// The cost of downloading 1 TB of data.
	DownloadTerabyte types.Currency `json:"downloadterabyte"`

	// The cost of forming a set of contracts using the defaults.
	FormContracts types.Currency `json:"formcontracts"`

	// The cost of storing 1 TB for a month, including redundancy.
	StorageTerabyteMonth types.Currency `json:"storageterabytemonth"`

	// The cost of consuming 1 TB of upload bandwidth from the host, including
	// redundancy.
	UploadTerabyte types.Currency `json:"uploadterabyte"`
}

// RenterSettings control the behavior of the Renter.
type RenterSettings struct {
	Allowance Allowance `json:"allowance"`
}

// HostDBScans represents a sortable slice of scans.
type HostDBScans []HostDBScan

func (s HostDBScans) Len() int           { return len(s) }
func (s HostDBScans) Less(i, j int) bool { return s[i].Timestamp.Before(s[j].Timestamp) }
func (s HostDBScans) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// MerkleRootSet is a set of Merkle roots, and gets encoded more efficiently.
type MerkleRootSet []crypto.Hash

// MarshalJSON defines a JSON encoding for a MerkleRootSet.
func (mrs MerkleRootSet) MarshalJSON() ([]byte, error) {
	// Copy the whole array into a giant byte slice and then encode that.
	fullBytes := make([]byte, crypto.HashSize*len(mrs))
	for i := range mrs {
		copy(fullBytes[i*crypto.HashSize:(i+1)*crypto.HashSize], mrs[i][:])
	}
	return json.Marshal(fullBytes)
}

// UnmarshalJSON attempts to decode a MerkleRootSet, falling back on the legacy
// decoding of a []crypto.Hash if that fails.
func (mrs *MerkleRootSet) UnmarshalJSON(b []byte) error {
	// Decode the giant byte slice, and then split it into separate arrays.
	var fullBytes []byte
	err := json.Unmarshal(b, &fullBytes)
	if err != nil {
		// Encoding the byte slice has failed, try decoding it as a []crypto.Hash.
		var hashes []crypto.Hash
		err := json.Unmarshal(b, &hashes)
		if err != nil {
			return err
		}
		*mrs = MerkleRootSet(hashes)
		return nil
	}

	umrs := make(MerkleRootSet, len(fullBytes)/32)
	for i := range umrs {
		copy(umrs[i][:], fullBytes[i*crypto.HashSize:(i+1)*crypto.HashSize])
	}
	*mrs = umrs
	return nil
}

// A RenterContract contains all the metadata necessary to revise or renew a
// file contract. See `api.RenterContract` for field information.
type RenterContract struct {
	FileContract    types.FileContract         `json:"filecontract"`
	HostPublicKey   types.SiaPublicKey         `json:"hostpublickey"`
	ID              types.FileContractID       `json:"id"`
	LastRevision    types.FileContractRevision `json:"lastrevision"`
	LastRevisionTxn types.Transaction          `json:"lastrevisiontxn"`
	MerkleRoots     MerkleRootSet              `json:"merkleroots"`
	NetAddress      NetAddress                 `json:"netaddress"`
	SecretKey       crypto.SecretKey           `json:"secretkey"`
	StartHeight     types.BlockHeight          `json:"startheight"`

	DownloadSpending types.Currency `json:"downloadspending"`
	StorageSpending  types.Currency `json:"storagespending"`
	UploadSpending   types.Currency `json:"uploadspending"`

	TotalCost   types.Currency `json:"totalcost"`
	ContractFee types.Currency `json:"contractfee"`
	TxnFee      types.Currency `json:"txnfee"`
	SiafundFee  types.Currency `json:"siafundfee"`
}

// EndHeight returns the height at which the host is no longer obligated to
// store contract data.
func (rc *RenterContract) EndHeight() types.BlockHeight {
	return rc.LastRevision.NewWindowStart
}

// RenterFunds returns the funds remaining in the contract's Renter payout as
// of the most recent revision.
func (rc *RenterContract) RenterFunds() types.Currency {
	if len(rc.LastRevision.NewValidProofOutputs) < 2 {
		build.Critical("malformed RenterContract:", rc)
		return types.ZeroCurrency
	}
	return rc.LastRevision.NewValidProofOutputs[0].Value
}

// A Renter uploads, tracks, repairs, and downloads a set of files for the
// user.
type Renter interface {
	// ActiveHosts provides the list of hosts that the renter is selecting,
	// sorted by preference.
	ActiveHosts() []HostDBEntry

	// AllHosts returns the full list of hosts known to the renter.
	AllHosts() []HostDBEntry

	// Close closes the Renter.
	Close() error

	// Contracts returns the contracts formed by the renter.
	Contracts() []RenterContract

	// CurrentPeriod returns the height at which the current allowance period
	// began.
	CurrentPeriod() types.BlockHeight

	// DeleteFile deletes a file entry from the renter.
	DeleteFile(path string) error

	// DownloadSection performs a download according to the parameters passed, including downloads of `offset` and `length` type.
	Download(params *RenterDownloadParameters) error

	// DownloadQueue lists all the files that have been scheduled for download.
	DownloadQueue() []DownloadInfo

	// FileList returns information on all of the files stored by the renter.
	FileList() []FileInfo

	// Host provides the DB entry and score breakdown for the requested host.
	Host(pk types.SiaPublicKey) (HostDBEntry, bool)

	// LoadSharedFiles loads a '.sia' file into the renter. A .sia file may
	// contain multiple files. The paths of the added files are returned.
	LoadSharedFiles(source string) ([]string, error)

	// LoadSharedFilesAscii loads an ASCII-encoded '.sia' file into the
	// renter.
	LoadSharedFilesAscii(asciiSia string) ([]string, error)

	// PriceEstimation estimates the cost in siacoins of performing various
	// storage and data operations.
	PriceEstimation() RenterPriceEstimation

	// RenameFile changes the path of a file.
	RenameFile(path, newPath string) error

	// ScoreBreakdown will return the score for a host db entry using the
	// hostdb's weighting algorithm.
	ScoreBreakdown(entry HostDBEntry) HostScoreBreakdown

	// Settings returns the Renter's current settings.
	Settings() RenterSettings

	// SetSettings sets the Renter's settings.
	SetSettings(RenterSettings) error

	// ShareFiles creates a '.sia' file that can be shared with others.
	ShareFiles(paths []string, shareDest string) error

	// ShareFilesAscii creates an ASCII-encoded '.sia' file.
	ShareFilesAscii(paths []string) (asciiSia string, err error)

	// Upload uploads a file using the input parameters.
	Upload(FileUploadParams) error
}

// RenterDownloadParameters contains all parameters that can be passed to the `/download` endpoint.
type RenterDownloadParameters struct {
	Async        bool
	DlWriter     DownloadWriter
	Httpresp     bool
	Length       uint64
	LengthPassed bool
	Offset       uint64
	OffsetPassed bool
	Siapath      string
}
