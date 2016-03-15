package api

import (
	"errors"
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
	defer signal.Reset(os.Interrupt, os.Kill)
	go func() {
		<-sigChan
		fmt.Println("\rCaught stop signal, quitting...")
		srv.listener.Close()
	}()

	var errStrs []string

	// The server will run until an error is encountered or the listener is
	// closed, via either the Close method or the signal handling above.
	// Closing the listener will result in the benign error handled below.
	err := srv.apiServer.Serve(srv.listener)
	if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
		errStrs = append(errStrs, fmt.Sprintf("serve err: %v", err))
	}

	// safely close each module
	if srv.host != nil {
		if err := srv.host.Close(); err != nil {
			errStrs = append(errStrs, fmt.Sprintf("host err: %v", err))
		}
	}
	// TODO: close renter (which should close hostdb as well)
	if srv.explorer != nil {
		if err := srv.explorer.Close(); err != nil {
			errStrs = append(errStrs, fmt.Sprintf("explorer err: %v", err))
		}
	}
	// TODO: close miner
	if srv.wallet != nil {
		// TODO: close wallet and lock the wallet in the wallet's Close method.
		if srv.wallet.Unlocked() {
			if err := srv.wallet.Lock(); err != nil {
				errStrs = append(errStrs, fmt.Sprintf("wallet err: %v", err))
			}
		}
	}
	// TODO: close transaction pool
	if srv.cs != nil {
		if err := srv.cs.Close(); err != nil {
			errStrs = append(errStrs, fmt.Sprintf("consensus err: %v", err))
		}
	}
	if srv.gateway != nil {
		if err := srv.gateway.Close(); err != nil {
			errStrs = append(errStrs, fmt.Sprintf("gateway err: %v", err))
		}
	}

	if len(errStrs) > 0 {
		return errors.New(strings.Join(errStrs, "\n"))
	}
	return nil
}

// Close closes the Server's listener, causing the HTTP server to shut down.
func (srv *Server) Close() error {
	return srv.listener.Close()
}
