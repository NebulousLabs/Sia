package siatest

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
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
}

// NewNode creates a new funded TestNode
func NewNode(nodeParams node.NodeParams) (*TestNode, error) {
	address := ":9980"
	userAgent := "Sia-Agent"
	password := "password"

	// We can't create a funded node without a miner
	if !nodeParams.CreateMiner && nodeParams.Miner == nil {
		return nil, errors.New("Can't create funded node without miner")
	}

	// Create client
	c := client.New(address)
	c.UserAgent = userAgent
	c.Password = password

	// Create server
	s, err := server.New(address, userAgent, password, nodeParams)
	if err != nil {
		return nil, err
	}

	// Encrypt and unlock wallet
	key := crypto.GenerateTwofishKey()
	_, err = s.Node.Wallet.Encrypt(key)
	if err != nil {
		return nil, err
	}
	if err := s.Node.Wallet.Unlock(key); err != nil {
		return nil, err
	}

	// fund the node
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := s.Node.Miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}

	// Return TestNode
	return &TestNode{*s, *c}, nil
}

// MineBlock makes the underlying node mine a single block and broadcast it.
func (tn *TestNode) MineBlock() error {
	if tn.Node.Miner == nil {
		return errors.New("server doesn't have the miner modules enabled")
	}
	if _, err := tn.Node.Miner.AddBlock(); err != nil {
		return build.ExtendErr("server failed to mine block:", err)
	}
	return nil
}
