package modules

import (
	"encoding/json"
	"io"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
)

// ErrHostFault is an error that is usually extended to indicate that an error
// is the host's fault.
var ErrHostFault = errors.New("host has returned an error")

// IsHostsFault indicates if a returned error is the host's fault.
func IsHostsFault(err error) bool {
	return errors.Contains(err, ErrHostFault)
}

const (
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

	// EncodeShards encodes the input data like Encode but accepts an already
	// sharded input.
	EncodeShards(data [][]byte) ([][]byte, error)

	// Recover recovers the original data from pieces and writes it to w.
	// pieces should be identical to the slice returned by Encode (length and
	// order must be preserved), but with missing elements set to nil. n is
	// the number of bytes to be written to w; this is necessary because
	// pieces may have been padded with zeros during encoding.
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

// ContractUtility contains metrics internal to the contractor that reflect the
// utility of a given contract.
type ContractUtility struct {
	GoodForUpload bool
	GoodForRenew  bool
}

// DownloadInfo provides information about a file that has been requested for
// download.
type DownloadInfo struct {
	Destination     string `json:"destination"`     // The destination of the download.
	DestinationType string `json:"destinationtype"` // Can be "file", "memory buffer", or "http stream".
	Length          uint64 `json:"length"`          // The length requested for the download.
	Offset          uint64 `json:"offset"`          // The offset within the siafile requested for the download.
	SiaPath         string `json:"siapath"`         // The siapath of the file used for the download.

	Completed            bool      `json:"completed"`            // Whether or not the download has completed.
	EndTime              time.Time `json:"endtime"`              // The time when the download fully completed.
	Error                string    `json:"error"`                // Will be the empty string unless there was an error.
	Received             uint64    `json:"received"`             // Amount of data confirmed and decoded.
	StartTime            time.Time `json:"starttime"`            // The time when the download was started.
	TotalDataTransferred uint64    `json:"totaldatatransferred"` // Total amount of data transferred, including negotiation, etc.
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
	LocalPath      string            `json:"localpath"`
	Filesize       uint64            `json:"filesize"`
	Available      bool              `json:"available"`
	Renewing       bool              `json:"renewing"`
	Redundancy     float64           `json:"redundancy"`
	UploadedBytes  uint64            `json:"uploadedbytes"`
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

	HistoricFailedInteractions     float64 `json:"historicfailedinteractions"`
	HistoricSuccessfulInteractions float64 `json:"historicsuccessfulinteractions"`
	RecentFailedInteractions       float64 `json:"recentfailedinteractions"`
	RecentSuccessfulInteractions   float64 `json:"recentsuccessfulinteractions"`

	LastHistoricUpdate types.BlockHeight

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
	Score          types.Currency `json:"score"`
	ConversionRate float64        `json:"conversionrate"`

	AgeAdjustment              float64 `json:"ageadjustment"`
	BurnAdjustment             float64 `json:"burnadjustment"`
	CollateralAdjustment       float64 `json:"collateraladjustment"`
	InteractionAdjustment      float64 `json:"interactionadjustment"`
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
	Allowance        Allowance `json:"allowance"`
	MaxUploadSpeed   int64     `json:"maxuploadspeed"`
	MaxDownloadSpeed int64     `json:"maxdownloadspeed"`
	StreamCacheSize  uint64    `json:"streamcachesize"`
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

// A RenterContract contains metadata about a file contract. It is read-only;
// modifying a RenterContract does not modify the actual file contract.
type RenterContract struct {
	ID            types.FileContractID
	HostPublicKey types.SiaPublicKey
	Transaction   types.Transaction

	StartHeight types.BlockHeight
	EndHeight   types.BlockHeight

	// RenterFunds is the amount remaining in the contract that the renter can
	// spend.
	RenterFunds types.Currency

	// The FileContract does not indicate what funds were spent on, so we have
	// to track the various costs manually.
	DownloadSpending types.Currency
	StorageSpending  types.Currency
	UploadSpending   types.Currency

	// Utility contains utility information about the renter.
	Utility ContractUtility

	// TotalCost indicates the amount of money that the renter spent and/or
	// locked up while forming a contract. This includes fees, and includes
	// funds which were allocated (but not necessarily committed) to spend on
	// uploads/downloads/storage.
	TotalCost types.Currency

	// ContractFee is the amount of money paid to the host to cover potential
	// future transaction fees that the host may incur, and to cover any other
	// overheads the host may have.
	//
	// TxnFee is the amount of money spent on the transaction fee when putting
	// the renter contract on the blockchain.
	//
	// SiafundFee is the amount of money spent on siafund fees when creating the
	// contract. The siafund fee that the renter pays covers both the renter and
	// the host portions of the contract, and therefore can be unexpectedly high
	// if the the host collateral is high.
	ContractFee types.Currency
	TxnFee      types.Currency
	SiafundFee  types.Currency
}

// ContractorSpending contains the metrics about how much the Contractor has
// spent during the current billing period.
type ContractorSpending struct {
	// ContractFees are the sum of all fees in the contract. This means it
	// includes the ContractFee, TxnFee and SiafundFee
	ContractFees types.Currency `json:"contractfees"`
	// DownloadSpending is the money currently spent on downloads.
	DownloadSpending types.Currency `json:"downloadspending"`
	// StorageSpending is the money currently spent on storage.
	StorageSpending types.Currency `json:"storagespending"`
	// ContractSpending is the total amount of money that the renter has put
	// into contracts, whether it's locked and the renter gets that money
	// back or whether it's spent and the renter won't get the money back.
	TotalAllocated types.Currency `json:"totalallocated"`
	// UploadSpending is the money currently spent on uploads.
	UploadSpending types.Currency `json:"uploadspending"`
	// Unspent is locked-away, unspent money.
	Unspent types.Currency `json:"unspent"`
	// ContractSpendingDeprecated was renamed to TotalAllocated and always has the
	// same value as TotalAllocated.
	ContractSpendingDeprecated types.Currency `json:"contractspending"`
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

	// ContractUtility provides the contract utility for a given host key.
	ContractUtility(pk types.SiaPublicKey) (ContractUtility, bool)

	// CurrentPeriod returns the height at which the current allowance period
	// began.
	CurrentPeriod() types.BlockHeight

	// PeriodSpending returns the amount spent on contracts in the current
	// billing period.
	PeriodSpending() ContractorSpending

	// DeleteFile deletes a file entry from the renter.
	DeleteFile(path string) error

	// Download performs a download according to the parameters passed, including
	// downloads of `offset` and `length` type.
	Download(params RenterDownloadParameters) error

	// Download performs a download according to the parameters passed without
	// blocking, including downloads of `offset` and `length` type.
	DownloadAsync(params RenterDownloadParameters) error

	// DownloadHistory lists all the files that have been scheduled for download.
	DownloadHistory() []DownloadInfo

	// File returns information on specific file queried by user
	File(siaPath string) (FileInfo, error)

	// FileList returns information on all of the files stored by the renter.
	FileList() []FileInfo

	// Host provides the DB entry and score breakdown for the requested host.
	Host(pk types.SiaPublicKey) (HostDBEntry, bool)

	// InitialScanComplete returns a boolean indicating if the initial scan of the
	// hostdb is completed.
	InitialScanComplete() (bool, error)

	// LoadSharedFiles loads a '.sia' file into the renter. A .sia file may
	// contain multiple files. The paths of the added files are returned.
	LoadSharedFiles(source string) ([]string, error)

	// LoadSharedFilesASCII loads an ASCII-encoded '.sia' file into the
	// renter.
	LoadSharedFilesASCII(asciiSia string) ([]string, error)

	// PriceEstimation estimates the cost in siacoins of performing various
	// storage and data operations.
	PriceEstimation() RenterPriceEstimation

	// RenameFile changes the path of a file.
	RenameFile(path, newPath string) error

	// EstimateHostScore will return the score for a host with the provided
	// settings, assuming perfect age and uptime adjustments
	EstimateHostScore(entry HostDBEntry) HostScoreBreakdown

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
	ShareFilesASCII(paths []string) (asciiSia string, err error)

	// Streamer creates a io.ReadSeeker that can be used to stream downloads
	// from the Sia network and also returns the fileName of the streamed
	// resource.
	Streamer(siaPath string) (string, io.ReadSeeker, error)

	// Upload uploads a file using the input parameters.
	Upload(FileUploadParams) error
}

// RenterDownloadParameters defines the parameters passed to the Renter's
// Download method.
type RenterDownloadParameters struct {
	Async       bool
	Httpwriter  io.Writer
	Length      uint64
	Offset      uint64
	SiaPath     string
	Destination string
}
