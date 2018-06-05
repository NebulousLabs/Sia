package siatest

import (
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api/client"
	"github.com/NebulousLabs/Sia/node/api/server"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
)

// TestNode is a helper struct for testing that contains a server and a client
// as embedded fields.
type TestNode struct {
	*server.Server
	client.Client
	params      node.NodeParams
	primarySeed string
}

// RestartNode restarts a TestNode
func (tn *TestNode) RestartNode() error {
	err := tn.StopNode()
	if err != nil {
		return errors.AddContext(err, "Could not stop node")
	}
	err = tn.StartNode()
	if err != nil {
		return errors.AddContext(err, "Could not start node")
	}
	return nil
}

// StartNode starts a TestNode from an active group
func (tn *TestNode) StartNode() error {
	// Create server
	s, err := server.New(":0", tn.UserAgent, tn.Password, tn.params)
	if err != nil {
		return err
	}
	tn.Server = s
	tn.Client.Address = s.APIAddress()
	return tn.WalletUnlockPost(tn.primarySeed)
}

// StopNode stops a TestNode
func (tn *TestNode) StopNode() error {
	return errors.AddContext(tn.Close(), "failed to stop node")
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
	for i := types.BlockHeight(0); i <= types.MaturityDelay+types.TaxHardforkHeight; i++ {
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
	tn := &TestNode{s, *c, nodeParams, ""}

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
