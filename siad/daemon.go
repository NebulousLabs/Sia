package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

// DaemonConfig is a struct containing the daemon configuration variables. It
// is only used when calling 'newDaemon', but is it's own struct because there
// are many values.
type DaemonConfig struct {
	APIAddr string
	RPCAddr string

	SiaDir string
}

// The daemon is essentially a collection of modules and an API server to talk
// to them all.
type daemon struct {
	state   *consensus.State
	gateway modules.Gateway
	host    modules.Host
	hostdb  modules.HostDB
	miner   modules.Miner
	renter  modules.Renter
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	apiServer *graceful.Server
}

// newDaemon will take the config struct and create a new daemon based on the
// parameters.
func newDaemon(config DaemonConfig) (d *daemon, err error) {
	d = new(daemon)

	// Create a folder for each module in siaDir.
	err = createSubdirs(config.SiaDir)
	if err != nil {
		return
	}

	d.state = consensus.CreateGenesisState()
	d.gateway, err = gateway.New(config.RPCAddr, d.state, filepath.Join(config.SiaDir, "gateway"))
	if err != nil {
		return
	}
	d.tpool, err = transactionpool.New(d.state, d.gateway)
	if err != nil {
		return
	}
	d.wallet, err = wallet.New(d.state, d.tpool, filepath.Join(config.SiaDir, "wallet"))
	if err != nil {
		return
	}
	d.miner, err = miner.New(d.state, d.gateway, d.tpool, d.wallet)
	if err != nil {
		return
	}
	d.host, err = host.New(d.state, d.tpool, d.wallet, filepath.Join(config.SiaDir, "host"))
	if err != nil {
		return
	}
	d.hostdb, err = hostdb.New(d.state, d.gateway)
	if err != nil {
		return
	}
	d.renter, err = renter.New(d.state, d.gateway, d.hostdb, d.wallet, filepath.Join(config.SiaDir, "renter"))
	if err != nil {
		return
	}

	// Register RPCs for each module
	d.gateway.RegisterRPC("AcceptBlock", d.acceptBlock)
	d.gateway.RegisterRPC("AcceptTransaction", d.acceptTransaction)
	d.gateway.RegisterRPC("HostSettings", d.host.Settings)
	d.gateway.RegisterRPC("NegotiateContract", d.host.NegotiateContract)
	d.gateway.RegisterRPC("RetrieveFile", d.host.RetrieveFile)

	d.initAPI(config.APIAddr)

	return
}

func createSubdirs(rootDir string) error {
	subdirs := []string{"gateway", "wallet", "host", "renter"}
	for _, subdir := range subdirs {
		err := os.MkdirAll(filepath.Join(rootDir, subdir), 0777)
		if err != nil {
			return err
		}
	}
	return nil
}

// TODO: move this to the state module?
func (d *daemon) acceptBlock(conn modules.NetConn) error {
	var b consensus.Block
	err := conn.ReadObject(&b, consensus.BlockSizeLimit)
	if err != nil {
		return err
	}

	err = d.state.AcceptBlock(b)
	if err == consensus.ErrOrphan {
		go d.gateway.Synchronize(conn.Addr())
		return err
	} else if err != nil {
		return err
	}

	// Check if b is in the current path.
	height, exists := d.state.HeightOfBlock(b.ID())
	if !exists {
		if consensus.DEBUG {
			panic("could not get the height of a block that did not return an error when being accepted into the state")
		}
		return errors.New("state malfunction")
	}
	currentPathBlock, exists := d.state.BlockAtHeight(height)
	if !exists || b.ID() != currentPathBlock.ID() {
		return errors.New("block added, but it does not extend the state height")
	}

	d.gateway.RelayBlock(b)
	return nil
}

// TODO: move this to the tpool module?
func (d *daemon) acceptTransaction(conn modules.NetConn) error {
	var t consensus.Transaction
	err := conn.ReadObject(&t, consensus.BlockSizeLimit)
	if err != nil {
		return err
	}
	return d.tpool.AcceptTransaction(t)
}
