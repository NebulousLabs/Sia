package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/NebulousLabs/Sia/modules"
)

// A Server is essentially a collection of modules and an API server to talk
// to them all.
type Server struct {
	cs       modules.ConsensusSet
	explorer modules.Explorer
	gateway  modules.Gateway
	host     modules.Host
	miner    modules.Miner
	renter   modules.Renter
	tpool    modules.TransactionPool
	wallet   modules.Wallet

	apiServer         *http.Server
	daemonExposed     bool
	listener          net.Listener
	requiredUserAgent string
}

// NewServer creates a new API server from the provided modules.
func NewServer(APIaddr string, requiredUserAgent string, cs modules.ConsensusSet, e modules.Explorer, g modules.Gateway, h modules.Host, m modules.Miner, r modules.Renter, tp modules.TransactionPool, w modules.Wallet) (*Server, error) {
	l, err := net.Listen("tcp", APIaddr)
	if err != nil {
		return nil, err
	}

	srv := &Server{
		cs:       cs,
		explorer: e,
		gateway:  g,
		host:     h,
		miner:    m,
		renter:   r,
		tpool:    tp,
		wallet:   w,

		listener:          l,
		requiredUserAgent: requiredUserAgent,
	}

	// Register API handlers
	srv.initAPI()

	return srv, nil
}

// Serve listens for and handles API calls. It a blocking function.
func (srv *Server) Serve() error {
	// stop the server if a kill signal is caught
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, os.Kill)
	go func() {
		<-sigChan
		fmt.Println("\rCaught stop signal, quitting...")
		srv.listener.Close()
	}()

	// The server will run until an error is encountered or the listener is
	// closed, via either the Close method or the signal handling above.
	// Closing the listener will result in the benign error handled below.
	err := srv.apiServer.Serve(srv.listener)
	if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
		return err
	}

	// safely close each module
	if srv.cs != nil {
		srv.cs.Close()
	}
	if srv.gateway != nil {
		srv.gateway.Close()
	}
	if srv.wallet != nil {
		srv.wallet.Lock()
	}

	return nil
}

// Close closes the Server's listener, causing the HTTP server to shut down.
func (srv *Server) Close() error {
	return srv.listener.Close()
}
