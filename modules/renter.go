package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// UploadParams contains the information used by the Renter to upload a file.
type UploadParams struct {
	Filename string
	Duration consensus.BlockHeight
	Nickname string
	Pieces   int
}

// FileInfo is an interface providing information about a file.
type FileInfo interface {
	// Available indicates whether the file is available for downloading or
	// not.
	Available() bool

	// Nickname gives the nickname of the file.
	Nickname() string

	// Repairing indicates whether the file is actively being repaired. If
	// there are files being repaired, it is best to let them finish before
	// shutting down the program.
	Repairing() bool

	// TimeRemaining indicates how many blocks remain before the file expires.
	TimeRemaining() consensus.BlockHeight
}

// DownloadInfo is an interface providing information about a file that has
// been requested for download.
type DownloadInfo interface {
	// Completed indicates whether the download has finished or not.
	Completed() bool

	// Destination is the filepath that the file was downloaded into.
	Destination() string

	// Nickname gives the name of the file according to the renter. Nickname
	// may be different from Destination.
	Nickname() string
}

// RentInfo contains a list of all files by nickname. (deprecated)
type RentInfo struct {
	Files []string
}

// A Renter uploads, tracks, repairs, and downloads a set of files for the
// user.
type Renter interface {
	// Download downloads a file to the given filepath.
	Download(nickname, filepath string) error

	// DownloadQueue lists all the files that have been scheduled for download.
	DownloadQueue() []DownloadInfo

	// FileList returns information on all of the files stored by the renter.
	FileList() []FileInfo

	// Info returns the list of all files by nickname. (deprecated)
	Info() RentInfo

	// Rename changes the nickname of a file.
	Rename(currentName, newName string) error

	// Upload uploads a file using the input parameters.
	Upload(UploadParams) error
}
