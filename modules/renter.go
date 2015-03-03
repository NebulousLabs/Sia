package modules

import (
	"io"

	"github.com/NebulousLabs/Sia/consensus"
)

// UploadParams contains the information used by the Renter to upload a file,
// including the file contents and the duration for which it is to be stored.
type UploadParams struct {
	Data     io.ReadSeeker
	Duration consensus.BlockHeight
	Nickname string
	Pieces   int
}

type RentInfo struct {
	Files []string
}

type Renter interface {
	Upload(UploadParams) error
	Download(nickname, filepath string) error
	Rename(currentName, newName string) error
	Info() RentInfo
}
