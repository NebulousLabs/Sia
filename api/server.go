package api

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/modules"

	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/threadgroup"
)

// A Server is a collection of siad modules that can be communicated with over
// an http api.
type Server struct {
	api               *API
	apiServer         *http.Server
	listener          net.Listener
	requiredUserAgent string
	tg                threadgroup.ThreadGroup
}

// Close closes the Server's listener, causing the HTTP server to shut down.
func (srv *Server) Close() error {
	err := srv.tg.Stop()
	if err != nil {
		return errors.AddContext(err, "unable to close server")
	}

	// Safely close each module.
	var errs []error
	mods := []struct {
		name string
		c    io.Closer
	}{
		{"explorer", srv.api.explorer},
		{"host", srv.api.host},
		{"renter", srv.api.renter},
		{"miner", srv.api.miner},
		{"wallet", srv.api.wallet},
		{"tpool", srv.api.tpool},
		{"consensus", srv.api.cs},
		{"gateway", srv.api.gateway},
	}
	for _, mod := range mods {
		if mod.c != nil {
			if err := mod.c.Close(); err != nil {
				errs = append(errs, fmt.Errorf("%v.Close failed: %v", mod.name, err))
			}
		}
	}
	return errors.Compose(errs...)
}

// Serve listens for and handles API calls. It is a blocking function.
func (srv *Server) Serve() error {
	err := srv.tg.Add()
	if err != nil {
		return errors.AddContext(err, "unable to initialize server")
	}
	defer srv.tg.Done()

	// The server will run until an error is encountered or the listener is
	// closed, via either the Close method or by signal handling.  Closing the
	// listener will result in the benign error handled below.
	err = srv.apiServer.Serve(srv.listener)
	if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
		return err
	}
	return nil
}

// NewServer creates a new API server from the provided modules. The API will
// require authentication using HTTP basic auth if the supplied password is not
// the empty string. Usernames are ignored for authentication. This type of
// authentication sends passwords in plaintext and should therefore only be
// used if the APIaddr is localhost.
func NewServer(APIaddr string, requiredUserAgent string, requiredPassword string, cs modules.ConsensusSet, e modules.Explorer, g modules.Gateway, h modules.Host, m modules.Miner, r modules.Renter, tp modules.TransactionPool, w modules.Wallet) (*Server, error) {
	var tg threadgroup.ThreadGroup
	listener, err := net.Listen("tcp", APIaddr)
	if err != nil {
		return nil, err
	}
	tg.OnStop(func() error {
		return errors.AddContext(listener.Close(), "unable to close server listener")
	})

	api := New(requiredUserAgent, requiredPassword, cs, e, g, h, m, r, tp, w)
	srv := &Server{
		api: api,

		listener:          listener,
		requiredUserAgent: requiredUserAgent,
		apiServer: &http.Server{
			Handler: api,
		},
	}
	return srv, nil
}
