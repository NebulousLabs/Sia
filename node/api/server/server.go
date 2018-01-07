// package server provides a server that can wrap a node and serve an http api
// for interacting with the node.
package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api"

	"github.com/NebulousLabs/errors"
)

// A Server is a collection of siad modules that can be communicated with over
// an http api.
type Server struct {
	api               *api.API
	apiServer         *http.Server
	errChan           chan error
	listener          net.Listener
	node              *node.Node
	requiredUserAgent string
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
	// Ordering is important. Listener must be closed before the threadgroup is
	// stopped. Then the threadgroup should be stopped before grabbing the error
	// from the errChan.
	err := srv.listener.Close()
	err = errors.Compose(err, <-srv.errChan)
	err = errors.Compose(err, srv.node.Close())
	return errors.AddContext(err, "error while closing server")
}

// New creates a new API server from the provided modules. The API will
// require authentication using HTTP basic auth if the supplied password is not
// the empty string. Usernames are ignored for authentication. This type of
// authentication sends passwords in plaintext and should therefore only be
// used if the APIaddr is localhost.
func New(APIaddr string, requiredUserAgent string, requiredPassword string, nodeParams node.NodeParams) (*Server, error) {
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
		listener:          listener,
		requiredUserAgent: requiredUserAgent,
	}

	// Spin up a channel that will block until the server has closed, and then
	// send any error down the channel.
	go func() {
		err := srv.serve()
		srv.errChan <- err
		close(srv.errChan)
	}()

	return srv, nil
}
