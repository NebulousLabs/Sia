package siatest

import (
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api/client"
	"github.com/NebulousLabs/Sia/node/api/server"
)

// NewClientServerPair creates a server and a client that points to the
// server's api.
func NewClientServerPair(params node.NodeParams) (*client.Client, *server.Server, error) {
	address := ":9980"
	userAgent := "Sia-Agent"
	password := "password"

	// Create server
	s, err := server.New(address, userAgent, password, params)
	if err != nil {
		return nil, nil, err
	}

	// Create client
	c := client.New(address)
	c.UserAgent = userAgent
	c.Password = password
	return c, s, err
}
