package api

import (
	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
)

// A Server is essentially a collection of modules and an API server to talk
// to them all.
type Server struct {
	cs      *consensus.State
	gateway modules.Gateway
	host    modules.Host
	hostdb  modules.HostDB
	miner   modules.Miner
	renter  modules.Renter
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	apiServer *graceful.Server
}

// NewServer creates a new API server from the provided modules.
func NewServer(APIaddr string, s *consensus.State, g modules.Gateway, h modules.Host, hdb modules.HostDB, m modules.Miner, r modules.Renter, tp modules.TransactionPool, w modules.Wallet) (*Server, error) {
	srv := &Server{
		cs:      s,
		gateway: g,
		host:    h,
		hostdb:  hdb,
		miner:   m,
		renter:  r,
		tpool:   tp,
		wallet:  w,
	}

	// Register API handlers
	srv.initAPI(APIaddr)

	return srv, nil
}
