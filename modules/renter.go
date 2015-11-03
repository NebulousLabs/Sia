package modules

import (
	"io"
	"time"

	"github.com/NebulousLabs/Sia/types"
)

var (
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
	Filename    string
	Duration    types.BlockHeight
	Nickname    string
	ErasureCode ErasureCoder
	PieceSize   uint64
}

// FileInfo is an interface providing information about a file.
type FileInfo interface {
	// Available indicates whether the file is available for downloading or
	// not.
	Available() bool

	// UploadProgress is a percentage indicating the progress of the file as
	// it is being uploaded. This percentage is calculated internally (unlike
	// DownloadInfo) because redundancy schemes complicate the definition of
	// "progress." Since UploadProgress includes redundancy, files will almost
	// certainly be Available before UploadProgress == 100.
	UploadProgress() float32

	// Nickname is the nickname of the file.
	Nickname() string

	// Filesize is the size of the file.
	Filesize() uint64

	// Expiration is the block height at which the file will expire.
	Expiration() types.BlockHeight
}

// DownloadInfo is an interface providing information about a file that has
// been requested for download.
type DownloadInfo interface {
	// StartTime is when the download was initiated.
	StartTime() time.Time

	// Filesize is the size of the file being downloaded.
	Filesize() uint64

	// Received is the number of bytes downloaded so far.
	Received() uint64

	// Destination is the filepath that the file was downloaded into.
	Destination() string

	// Nickname is the identifier assigned to the file when it was uploaded.
	Nickname() string
}

// RentInfo contains a list of all files by nickname. (deprecated)
type RentInfo struct {
	Files      []string
	Price      types.Currency
	KnownHosts int
}

// A Renter uploads, tracks, repairs, and downloads a set of files for the
// user.
type Renter interface {
	// ActiveHosts returns the list of hosts that are actively being selected
	// from.
	ActiveHosts() []HostSettings

	// AllHosts returns the full list of hosts known to the renter.
	AllHosts() []HostSettings

	// DeleteFile deletes a file entry from the renter.
	DeleteFile(nickname string) error

	// Download downloads a file to the given filepath.
	Download(nickname, filepath string) error

	// DownloadQueue lists all the files that have been scheduled for download.
	DownloadQueue() []DownloadInfo

	// FileList returns information on all of the files stored by the renter.
	FileList() []FileInfo

	// Info returns the list of all files by nickname. (deprecated)
	Info() RentInfo

	// LoadSharedFiles loads a '.sia' file into the renter. A .sia file may
	// contain multiple files. The nicknames of the added files are returned.
	LoadSharedFiles(filename string) ([]string, error)

	// LoadSharedFilesAscii loads an ASCII-encoded '.sia' file into the
	// renter.
	LoadSharedFilesAscii(asciiSia string) ([]string, error)

	// Rename changes the nickname of a file.
	RenameFile(currentName, newName string) error

	// ShareFiles creates a '.sia' file that can be shared with others.
	ShareFiles(nicknames []string, shareDest string) error

	// ShareFilesAscii creates an ASCII-encoded '.sia' file.
	ShareFilesAscii(nicknames []string) (asciiSia string, err error)

	// Upload uploads a file using the input parameters.
	Upload(FileUploadParams) error
}
