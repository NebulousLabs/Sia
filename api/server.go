package api

import (
	"errors"

	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/types"
)

// A Server is essentially a collection of modules and an API server to talk
// to them all.
type Server struct {
	state   *consensus.State
	gateway modules.Gateway
	host    modules.Host
	hostdb  modules.HostDB
	miner   modules.Miner
	renter  modules.Renter
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	minerUpdates  <-chan struct{}
	walletUpdates <-chan struct{}

	apiServer *graceful.Server
}

// NewServer creates a new API server from the provided modules.
func NewServer(APIAddr string, s *consensus.State, g modules.Gateway, h modules.Host, hdb modules.HostDB, m modules.Miner, r modules.Renter, tp modules.TransactionPool, w modules.Wallet) *Server {
	srv := &Server{
		state:   s,
		gateway: g,
		host:    h,
		hostdb:  hdb,
		miner:   m,
		renter:  r,
		tpool:   tp,
		wallet:  w,
	}

	// Subscribe to the miner and wallet
	srv.minerUpdates = srv.miner.MinerSubscribe()
	srv.walletUpdates = srv.wallet.WalletSubscribe()

	// Register RPCs for each module
	g.RegisterRPC("AcceptBlock", srv.acceptBlock)
	g.RegisterRPC("AcceptTransaction", srv.acceptTransaction)
	g.RegisterRPC("HostSettings", h.Settings)
	g.RegisterRPC("NegotiateContract", h.NegotiateContract)
	g.RegisterRPC("RetrieveFile", h.RetrieveFile)

	// Register API handlers
	srv.initAPI(APIAddr)

	return srv
}

func (srv *Server) updateWait() {
	<-srv.minerUpdates
	<-srv.walletUpdates
}

// TODO: move this to the state module?
func (srv *Server) acceptBlock(conn modules.NetConn) error {
	var b types.Block
	err := conn.ReadObject(&b, types.BlockSizeLimit)
	if err != nil {
		return err
	}

	err = srv.state.AcceptBlock(b)
	if err == consensus.ErrOrphan {
		go srv.gateway.Synchronize(conn.Addr())
		return err
	} else if err != nil {
		return err
	}

	// Check if b is in the current path.
	height, exists := srv.state.HeightOfBlock(b.ID())
	if !exists {
		if build.DEBUG {
			panic("could not get the height of a block that did not return an error when being accepted into the state")
		}
		return errors.New("state malfunction")
	}
	currentPathBlock, exists := srv.state.BlockAtHeight(height)
	if !exists || b.ID() != currentPathBlock.ID() {
		return errors.New("block added, but it does not extend the state height")
	}

	srv.gateway.RelayBlock(b)
	return nil
}

// TODO: move this to the tpool module?
func (srv *Server) acceptTransaction(conn modules.NetConn) error {
	var t types.Transaction
	err := conn.ReadObject(&t, types.BlockSizeLimit)
	if err != nil {
		return err
	}
	return srv.tpool.AcceptTransaction(t)
}
