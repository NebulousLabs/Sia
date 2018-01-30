// Package server provides a server that can wrap a node and serve an http api
// for interacting with the node.
package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
)

// A Server is a collection of siad modules that can be communicated with over
// an http api.
type Server struct {
	api               *api.API
	apiServer         *http.Server
	done              chan struct{}
	listener          net.Listener
	node              *node.Node
	requiredUserAgent string
	serveErr          error
}

// serve listens for and handles API calls. It is a blocking function.
func (srv *Server) serve() error {
	// The server will run until an error is encountered or the listener is
	// closed, via either the Close method or by signal handling.  Closing the
	// listener will result in the benign error handled below.
	err := srv.apiServer.Serve(srv.listener)
	if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
		return err
	}
	return nil
}

// Close closes the Server's listener, causing the HTTP server to shut down.
func (srv *Server) Close() error {
	// Stop accepting API requests.
	err := srv.listener.Close()
	// Wait for serve() to return and capture its error.
	<-srv.done
	err = errors.Compose(err, srv.serveErr)
	// Shutdown modules.
	err = errors.Compose(err, srv.node.Close())
	return errors.AddContext(err, "error while closing server")
}

// New creates a new API server from the provided modules. The API will
// require authentication using HTTP basic auth if the supplied password is not
// the empty string. Usernames are ignored for authentication. This type of
// authentication sends passwords in plaintext and should therefore only be
// used if the APIaddr is localhost.
func New(APIaddr string, requiredUserAgent string, requiredPassword string, nodeParams node.NodeParams) (*Server, error) {
	// We can't create a funded node without a miner
	if !nodeParams.CreateMiner && nodeParams.Miner == nil {
		return nil, errors.New("Can't create funded node without miner")
	}

	// Create the server listener.
	listener, err := net.Listen("tcp", APIaddr)
	if err != nil {
		return nil, err
	}

	// Create the Sia node for the server.
	node, err := node.New(nodeParams)
	if err != nil {
		return nil, errors.AddContext(err, "server is unable to create the Sia node")
	}

	// Create the api for the server.
	api := api.New(requiredUserAgent, requiredPassword, node.ConsensusSet, node.Explorer, node.Gateway, node.Host, node.Miner, node.Renter, node.TransactionPool, node.Wallet)
	srv := &Server{
		api: api,
		apiServer: &http.Server{
			Handler: api,
		},
		done:              make(chan struct{}),
		listener:          listener,
		node:              node,
		requiredUserAgent: requiredUserAgent,
	}

	// fund the node
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := node.Miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}

	// Spin up a goroutine that serves the API and closes srv.done when
	// finished.
	go func() {
		srv.serveErr = srv.serve()
		close(srv.done)
	}()

	return srv, nil
}

// MineBlock makes the underlying node mine a single block and broadcast it.
func (srv *Server) MineBlock() error {
	if srv.node.Miner == nil {
		return errors.New("server doesn't have the miner modules enabled")
	}
	if _, err := srv.node.Miner.AddBlock(); err != nil {
		return build.ExtendErr("server failed to mine block:", err)
	}
	return nil
}
