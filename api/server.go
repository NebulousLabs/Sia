package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/NebulousLabs/Sia/build"
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

	// wg is used to block Close() from returning until Serve() has finished. A
	// WaitGroup is used instead of a chan struct{} so that Close() can be called
	// without necessarily calling Serve() first.
	wg sync.WaitGroup
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

// Serve listens for and handles API calls. It is a blocking function.
func (srv *Server) Serve() error {
	// Block the Close() method until Serve() has finished.
	srv.wg.Add(1)
	defer srv.wg.Done()

	// stop the server if a kill signal is caught
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill)
	defer signal.Stop(sigChan)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-sigChan:
			fmt.Println("\rCaught stop signal, quitting...")
			srv.Close()
		case <-stop:
			// Don't leave a dangling goroutine.
		}
	}()

	// The server will run until an error is encountered or the listener is
	// closed, via either the Close method or the signal handling above.
	// Closing the listener will result in the benign error handled below.
	err := srv.apiServer.Serve(srv.listener)
	if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
		return err
	}
	return nil
}

// Close closes the Server's listener, causing the HTTP server to shut down.
func (srv *Server) Close() error {
	var errs []error

	// Close the listener, which will cause Server.Serve() to return.
	if err := srv.listener.Close(); err != nil {
		errs = append(errs, fmt.Errorf("listener.Close failed: %v", err))
	}

	// Wait for Server.Serve() to exit. We wait so that it's guaranteed that the
	// server has completely closed after Close() returns. This is particularly
	// useful during testing so that we don't exit a test before Serve() finishes.
	srv.wg.Wait()

	// Safely close each module.
	if srv.host != nil {
		if err := srv.host.Close(); err != nil {
			errs = append(errs, fmt.Errorf("host.Close failed: %v", err))
		}
	}
	// TODO: close renter (which should close hostdb as well)
	if srv.explorer != nil {
		if err := srv.explorer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("explorer.Close failed: %v", err))
		}
	}
	if srv.miner != nil {
		if err := srv.miner.Close(); err != nil {
			errs = append(errs, fmt.Errorf("miner.Close failed: %v", err))
		}
	}
	if srv.wallet != nil {
		// TODO: close wallet and lock the wallet in the wallet's Close method.
		if srv.wallet.Unlocked() {
			if err := srv.wallet.Lock(); err != nil {
				errs = append(errs, fmt.Errorf("wallet.Lock failed: %v", err))
			}
		}
	}
	// TODO: close transaction pool
	if srv.cs != nil {
		if err := srv.cs.Close(); err != nil {
			errs = append(errs, fmt.Errorf("consensusset.Close failed: %v", err))
		}
	}
	if srv.gateway != nil {
		if err := srv.gateway.Close(); err != nil {
			errs = append(errs, fmt.Errorf("gateway.Close failed: %v", err))
		}
	}

	return build.JoinErrors(errs, "\n")
}
