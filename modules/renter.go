package modules

import (
	"time"

	"github.com/NebulousLabs/Sia/types"
)

var (
	RenterDir = "renter"
)

// FileUploadParams contains the information used by the Renter to upload a
// file.
type FileUploadParams struct {
	Filename string
	Duration types.BlockHeight
	Nickname string
	Pieces   int
}

// FileInfo is an interface providing information about a file.
type FileInfo interface {
	// Available indicates whether the file is available for downloading or
	// not.
	Available() bool

	// UploadProgress is a percentage indicating the progress of the file as
	// it is being uploaded. This percentage is calculated internally (unlike
	// DownloadInfo) because redundancy schemes complicate the definition of
	// "progress." As a rule, Available == true IFF UploadProgress == 100.0.
	UploadProgress() float32

	// Nickname gives the nickname of the file.
	Nickname() string

	// Repairing indicates whether the file is actively being repaired. If
	// there are files being repaired, it is best to let them finish before
	// shutting down the program.
	Repairing() bool

	// TimeRemaining indicates how many blocks remain before the file expires.
	TimeRemaining() types.BlockHeight
}

// DownloadInfo is an interface providing information about a file that has
// been requested for download.
type DownloadInfo interface {
	// StartTime is when the download was initiated.
	StartTime() time.Time

	// Complete returns whether the file is ready to be used. Note that
	// Received == Filesize does not imply Complete, because the file may
	// require additional processing (e.g. decryption) after all of the raw
	// bytes have been downloaded.
	Complete() bool

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

	// LoadSharedFile loads a '.sia' file into the renter, so that the user can
	// download files which have been shared with them.
	LoadSharedFile(filename string) ([]string, error)

	// LoadSharedFilesAscii loads a '.sia' file into the renter, except instead
	// of taking a filename it takes a base64 encoded string of the file.
	LoadSharedFilesAscii(asciiSia string) ([]string, error)

	// Rename changes the nickname of a file.
	RenameFile(currentName, newName string) error

	// RenterNotify will push a struct down the channel every time it receives
	// an update.
	RenterNotify() <-chan struct{}

	// ShareFiles creates a '.sia' file that can be shared with others, so that
	// they may download files which they have not uploaded.
	ShareFiles(nicknames []string, sharedest string) error

	// ShareFilesAscii creates a '.sia' file that can be shared with others,
	// except it returns the bytes of the file in base64.
	ShareFilesAscii(nicknames []string) (asciiSia string, err error)

	// Upload uploads a file using the input parameters.
	Upload(FileUploadParams) error
}
