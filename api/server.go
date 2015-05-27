package api

import (
	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// A Server is essentially a collection of modules and an API server to talk
// to them all.
type Server struct {
	cs      modules.ConsensusSet
	gateway modules.Gateway
	host    modules.Host
	hostdb  modules.HostDB
	miner   modules.Miner
	renter  modules.Renter
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	// Consensus set variables.
	blockchainHeight types.BlockHeight
	currentBlock     types.Block

	apiServer *graceful.Server

	mu *sync.RWMutex
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

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Set the genesis block and start listening to the consensus package.
	srv.currentBlock = srv.cs.GenesisBlock()
	srv.cs.ConsensusSetSubscribe(srv)

	// Register API handlers
	srv.initAPI(APIaddr)

	return srv, nil
}
