package siatest

import (
	"bytes"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api/client"
	"github.com/NebulousLabs/Sia/node/api/server"
	"github.com/NebulousLabs/Sia/types"
)

// TestNode is a helper struct for testing that contains a server and a client
// as embedded fields.
type TestNode struct {
	server.Server
	client.Client
	primarySeed string
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

// MineBlock makes the underlying node mine a single block and broadcast it.
func (tn *TestNode) MineBlock() error {
	// Get the header
	target, header, err := tn.MinerHeaderGet()
	if err != nil {
		return build.ExtendErr("failed to get header for work", err)
	}
	// Solve the header
	header, err = solveHeader(target, header)
	if err != nil {
		return build.ExtendErr("failed to solve header", err)
	}
	// Submit the header
	if err := tn.MinerHeaderPost(header); err != nil {
		return build.ExtendErr("failed to submit header", err)
	}
	return nil
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

// solveHeader solves the header by finding a nonce for the target
func solveHeader(target types.Target, bh types.BlockHeader) (types.BlockHeader, error) {
	header := encoding.Marshal(bh)
	var nonce uint64
	for i := 0; i < 256; i++ {
		id := crypto.HashBytes(header)
		if bytes.Compare(target[:], id[:]) >= 0 {
			copy(bh.Nonce[:], header[32:40])
			return bh, nil
		}
		*(*uint64)(unsafe.Pointer(&header[32])) = nonce
		nonce++
	}
	return bh, errors.New("couldn't solve block")
}
