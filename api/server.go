package api

import (
	"fmt"
	"net"
	"strings"

	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
)

// A Server is essentially a collection of modules and an API server to talk
// to them all.
type Server struct {
	cs       modules.ConsensusSet
	gateway  modules.Gateway
	host     modules.Host
	miner    modules.Miner
	renter   modules.Renter
	tpool    modules.TransactionPool
	wallet   modules.Wallet
	explorer modules.Explorer

	listener  net.Listener
	apiServer *graceful.Server

	mu *sync.RWMutex
}

// NewServer creates a new API server from the provided modules.
func NewServer(APIaddr string, s *consensus.ConsensusSet, g modules.Gateway, h modules.Host, m modules.Miner, r modules.Renter, tp modules.TransactionPool, w modules.Wallet, explorer modules.Explorer) (*Server, error) {
	l, err := net.Listen("tcp", APIaddr)
	if err != nil {
		return nil, err
	}

	srv := &Server{
		cs:       s,
		gateway:  g,
		host:     h,
		miner:    m,
		renter:   r,
		tpool:    tp,
		wallet:   w,
		explorer: explorer,

		listener: l,

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Register API handlers
	srv.initAPI()

	return srv, nil
}

// Serve listens for and handles API calls. It a blocking function.
func (srv *Server) Serve() error {
	// graceful will run until it catches a signal.
	// It can also be stopped manually by stopHandler.
	err := srv.apiServer.Serve(srv.listener)
	// despite its name, graceful still propogates this benign error
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

	fmt.Println("\rCaught stop signal, quitting.")
	return nil
}
