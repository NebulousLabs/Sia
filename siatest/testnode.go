package siatest

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/node/api/client"
	"github.com/NebulousLabs/Sia/node/api/server"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// TestNode is a helper struct for testing that contains a server and a client
// as embedded fields.
type TestNode struct {
	server.Server
	client.Client
	primarySeed string
}

// DownloadToDisk downloads a previously uploaded file. The file will be downloaded
// to a random location and returned as a TestFile object.
func (tn *TestNode) DownloadToDisk(tf *TestFile, async bool) (*TestFile, error) {
	fi, err := tn.FileInfo(tf)
	if err != nil {
		return nil, build.ExtendErr("failed to retrieve FileInfo", err)
	}
	// Create a random destination for the download
	fileName := strconv.Itoa(fastrand.Intn(math.MaxInt32))
	dest := filepath.Join(SiaTestingDir, fileName)
	if err := tn.RenterDownloadGet(tf.siaPath, dest, 0, fi.Filesize, async); err != nil {
		return nil, build.ExtendErr("failed to download file", err)
	}
	return &TestFile{
		path:     dest,
		fileName: fileName,
		siaPath:  tf.siaPath,
	}, nil
}

// Download downloads a file and returns its contents as a slice of bytes.
func (tn *TestNode) Download(tf *TestFile) (data []byte, err error) {
	fi, err := tn.FileInfo(tf)
	if err != nil {
		return nil, build.ExtendErr("failed to retrieve FileInfo", err)
	}
	data, err = tn.RenterDownloadHTTPResponseGet(tf.siaPath, 0, fi.Filesize)
	return
}

// DownloadInfo returns the DownloadInfo struct of a file. If it returns nil,
// the download has either finished, or was never started in the first place.
func (tn *TestNode) DownloadInfo(tf *TestFile) (*api.DownloadInfo, error) {
	rdq, err := tn.RenterDownloadsGet()
	if err != nil {
		return nil, err
	}
	for _, d := range rdq.Downloads {
		if tf.siaPath == d.SiaPath && tf.path == d.Destination {
			return &d, nil
		}
	}
	return nil, nil
}

// Files lists the files tracked by the renter
func (tn *TestNode) Files() ([]modules.FileInfo, error) {
	rf, err := tn.RenterFilesGet()
	if err != nil {
		return nil, err
	}
	return rf.Files, err
}

// FileInfo retrieves the info of a certain file that is tracked by the renter
func (tn *TestNode) FileInfo(tf *TestFile) (modules.FileInfo, error) {
	files, err := tn.Files()
	if err != nil {
		return modules.FileInfo{}, err
	}
	for _, file := range files {
		if file.SiaPath == tf.siaPath {
			return file, nil
		}
	}
	return modules.FileInfo{}, errors.New("file is not tracked by the renter")
}

// Upload uses the node to upload the file.
func (tn *TestNode) Upload(tf *TestFile, dataPieces, parityPieces uint64) (err error) {
	// Upload file
	err = tn.RenterUploadPost(tf.path, "/"+tf.fileName, dataPieces, parityPieces)
	if err != nil {
		return err
	}
	// Make sure renter tracks file
	_, err = tn.FileInfo(tf)
	if err != nil {
		return build.ExtendErr("uploaded file is not tracked by the renter", err)
	}
	return nil
}

// UploadNewFile initiates the upload of a filesize bytes large file.
func (tn *TestNode) UploadNewFile(filesize int, dataPieces uint64, parityPieces uint64) (file *TestFile, err error) {
	// Create file for upload
	file, err = NewFile(filesize)
	if err != nil {
		err = build.ExtendErr("failed to create file", err)
		return
	}
	// Upload file, creating a parity piece for each host in the group
	err = tn.Upload(file, dataPieces, parityPieces)
	if err != nil {
		err = build.ExtendErr("failed to start upload", err)
		return
	}
	return
}

// UploadNewFileBlocking uploads a filesize bytes large file and waits for the
// upload to reach 100% progress and redundancy.
func (tn *TestNode) UploadNewFileBlocking(filesize int, dataPieces uint64, parityPieces uint64) (file *TestFile, err error) {
	file, err = tn.UploadNewFile(filesize, dataPieces, parityPieces)
	if err != nil {
		return
	}
	// Wait until upload reached the specified progress
	if err = tn.WaitForUploadProgress(file, 1); err != nil {
		return
	}
	// Wait until upload reaches a certain redundancy
	err = tn.WaitForUploadRedundancy(file, float64((dataPieces+parityPieces))/float64(dataPieces))
	return
}

// WaitForDownload waits for the download of a file to finish. If a file wasn't
// scheduled for download it will return instantly without an error.
func (tn *TestNode) WaitForDownload(tf *TestFile) error {
	return Retry(1000, 100*time.Millisecond, func() error {
		file, err := tn.DownloadInfo(tf)
		if err != nil {
			return build.ExtendErr("couldn't retrieve DownloadInfo", err)
		}
		if file == nil {
			return nil
		}
		if file.Filesize != file.Received {
			return errors.New("file hasn't finished downloading yet")
		}
		return nil
	})
}

// WaitForUploadProgress waits for a file to reach a certain upload progress.
func (tn *TestNode) WaitForUploadProgress(tf *TestFile, progress float64) error {
	// Check if file is tracked by renter at all
	if _, err := tn.FileInfo(tf); err != nil {
		return errors.New("file is not tracked by renter")
	}
	// Wait until it reaches the progress
	return Retry(1000, 100*time.Millisecond, func() error {
		file, err := tn.FileInfo(tf)
		if err != nil {
			return build.ExtendErr("couldn't retrieve FileInfo", err)
		}
		if file.UploadProgress < progress {
			return fmt.Errorf("progress should be %v but was %v", progress, file.UploadProgress)
		}
		return nil
	})

}

// WaitForUploadRedundancy waits for a file to reach a certain upload redundancy.
func (tn *TestNode) WaitForUploadRedundancy(tf *TestFile, redundancy float64) error {
	// Check if file is tracked by renter at all
	if _, err := tn.FileInfo(tf); err != nil {
		return errors.New("file is not tracked by renter")
	}
	// Wait until it reaches the redundancy
	return Retry(1000, 100*time.Millisecond, func() error {
		file, err := tn.FileInfo(tf)
		if err != nil {
			return build.ExtendErr("couldn't retrieve FileInfo", err)
		}
		if file.Redundancy < redundancy {
			return fmt.Errorf("redundancy should be %v but was %v", redundancy, file.Redundancy)
		}
		return nil
	})
}

// NewNode creates a new funded TestNode
func NewNode(nodeParams node.NodeParams) (*TestNode, error) {
	// We can't create a funded node without a miner
	if !nodeParams.CreateMiner && nodeParams.Miner == nil {
		return nil, errors.New("Can't create funded node without miner")
	}
	// Create clean node
	tn, err := NewCleanNode(nodeParams)
	if err != nil {
		return nil, err
	}
	// Fund the node
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		if err := tn.MineBlock(); err != nil {
			return nil, err
		}
	}
	// Return TestNode
	return tn, nil
}

// NewCleanNode creates a new TestNode that's not yet funded
func NewCleanNode(nodeParams node.NodeParams) (*TestNode, error) {
	userAgent := "Sia-Agent"
	password := "password"

	// Create server
	s, err := server.New(":0", userAgent, password, nodeParams)
	if err != nil {
		return nil, err
	}

	// Create client
	c := client.New(s.APIAddress())
	c.UserAgent = userAgent
	c.Password = password

	// Create TestNode
	tn := &TestNode{*s, *c, ""}

	// Init wallet
	wip, err := tn.WalletInitPost("", false)
	if err != nil {
		return nil, err
	}
	tn.primarySeed = wip.PrimarySeed

	// Unlock wallet
	if err := tn.WalletUnlockPost(tn.primarySeed); err != nil {
		return nil, err
	}

	// Return TestNode
	return tn, nil
}
