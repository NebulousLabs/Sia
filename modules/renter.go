package modules

import (
	"io"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

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

	// Recover recovers the original data from pieces (including parity) and
	// writes it to w. pieces should be identical to the slice returned by
	// Encode (length and order must be preserved), but with missing elements
	// set to nil. n is the number of bytes to be written to w; this is
	// necessary because pieces may have been padded with zeros during
	// encoding.
	Recover(pieces [][]byte, n uint64, w io.Writer) error
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

// DownloadInfo provides information about a file that has been requested for
// download.
type DownloadInfo struct {
	SiaPath     string    `json:"siapath"`
	Destination string    `json:"destination"`
	Filesize    uint64    `json:"filesize"`
	Received    uint64    `json:"received"`
	StartTime   time.Time `json:"starttime"`
}

// An Allowance dictates how much the Renter is allowed to spend in a given
// period. Note that funds are spent on both storage and bandwidth.
type Allowance struct {
	Funds       types.Currency    `json:"funds"`
	Hosts       uint64            `json:"hosts"`
	Period      types.BlockHeight `json:"period"`
	RenewWindow types.BlockHeight `json:"renewwindow"`
}

// RenterSettings control the behavior of the Renter.
type RenterSettings struct {
	Allowance Allowance `json:"allowance"`
}

// A HostDBEntry represents one host entry in the Renter's host DB. It
// aggregates the host's external settings and metrics with its public key.
type HostDBEntry struct {
	HostExternalSettings
	PublicKey types.SiaPublicKey `json:"publickey"`
	// ScanHistory is the set of scans performed on the host. It should always
	// be ordered according to the scan's Timestamp, oldest to newest.
	ScanHistory HostDBScans
}

// HostDBScan represents a single scan event.
type HostDBScan struct {
	Timestamp time.Time
	Success   bool
}

// HostDBScans represents a sortable slice of scans.
type HostDBScans []HostDBScan

func (s HostDBScans) Len() int           { return len(s) }
func (s HostDBScans) Less(i, j int) bool { return s[i].Timestamp.Before(s[j].Timestamp) }
func (s HostDBScans) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// A RenterContract contains all the metadata necessary to revise or renew a
// file contract.
type RenterContract struct {
	FileContract    types.FileContract         `json:"filecontract"`
	ID              types.FileContractID       `json:"id"`
	LastRevision    types.FileContractRevision `json:"lastrevision"`
	LastRevisionTxn types.Transaction          `json:"lastrevisiontxn"`
	MerkleRoots     []crypto.Hash              `json:"merkleroots"`
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

	// Download downloads a file to the given destination.
	Download(path, destination string) error

	// DownloadQueue lists all the files that have been scheduled for download.
	DownloadQueue() []DownloadInfo

	// FileList returns information on all of the files stored by the renter.
	FileList() []FileInfo

	// LoadSharedFiles loads a '.sia' file into the renter. A .sia file may
	// contain multiple files. The paths of the added files are returned.
	LoadSharedFiles(source string) ([]string, error)

	// LoadSharedFilesAscii loads an ASCII-encoded '.sia' file into the
	// renter.
	LoadSharedFilesAscii(asciiSia string) ([]string, error)

	// RenameFile changes the path of a file.
	RenameFile(path, newPath string) error

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
