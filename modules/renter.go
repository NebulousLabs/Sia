package modules

import (
	"io"
	"time"

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
	Duration    types.BlockHeight
	Renew       bool
	ErasureCode ErasureCoder
	PieceSize   uint64
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
// period.
type Allowance struct {
	Funds       types.Currency    `json:"funds"`
	Hosts       uint64            `json:"hosts"`
	Period      types.BlockHeight `json:"period"`
	RenewWindow types.BlockHeight `json:"renewwindow"`
}

// A Renter uploads, tracks, repairs, and downloads a set of files for the
// user.
type Renter interface {
	// Allowance returns the current allowance.
	Allowance() Allowance

	// ActiveHosts returns the list of hosts that are actively being selected
	// from.
	ActiveHosts() []HostSettings

	// AllHosts returns the full list of hosts known to the renter.
	AllHosts() []HostSettings

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

	// Rename changes the path of a file.
	RenameFile(path, newPath string) error

	// SetAllowance sets the amount of money the Renter is allowed to spend on
	// contracts over a given time period, divided among the number of hosts
	// specified. Note that Renter can start forming contracts as soon as
	// SetAllowance is called; that is, it may block.
	SetAllowance(Allowance) error

	// ShareFiles creates a '.sia' file that can be shared with others.
	ShareFiles(paths []string, shareDest string) error

	// ShareFilesAscii creates an ASCII-encoded '.sia' file.
	ShareFilesAscii(paths []string) (asciiSia string, err error)

	// Upload uploads a file using the input parameters.
	Upload(FileUploadParams) error
}
