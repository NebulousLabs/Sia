package main

import (
	"errors"
	"html/template"
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia"
	"github.com/NebulousLabs/Sia/sia/host"
	"github.com/NebulousLabs/Sia/sia/hostdb"
	"github.com/NebulousLabs/Sia/sia/miner"
	"github.com/NebulousLabs/Sia/sia/renter"
	"github.com/NebulousLabs/Sia/sia/wallet"
)

type daemon struct {
	core *sia.Core

	// Modules. TODO: Implement all of them. So far it's just the miner.
	state  *consensus.State
	wallet *wallet.Wallet
	miner  *miner.Miner

	styleDir    string
	downloadDir string

	template *template.Template
}

func startDaemon(config Config) (err error) {
	// Create download directory and host directory.
	if err = os.MkdirAll(config.Siad.DownloadDirectory, os.ModeDir|os.ModePerm); err != nil {
		return errors.New("failed to create download directory: " + err.Error())
	}
	if err = os.MkdirAll(config.Siacore.HostDirectory, os.ModeDir|os.ModePerm); err != nil {
		return errors.New("failed to create host directory: " + err.Error())
	}

	// Create and fill out the daemon object.
	d := &daemon{
		styleDir:    config.Siad.StyleDirectory,
		downloadDir: config.Siad.DownloadDirectory,
	}

	d.state, _ = consensus.CreateGenesisState() // the `_` is not of type error. TODO: Deprecate this.
	d.wallet, err = wallet.New(d.state, config.Siad.WalletFile)
	if err != nil {
		return
	}
	d.miner, err = miner.New(d.state, d.wallet)
	if err != nil {
		return
	}
	hostDB, err := hostdb.New()
	if err != nil {
		return errors.New("could not load wallet file: " + err.Error())
	}
	Host, err := host.New(d.state, d.wallet)
	if err != nil {
		return
	}
	Renter, err := renter.New(d.state, hostDB, d.wallet)
	if err != nil {
		return
	}

	siaconfig := sia.Config{
		HostDir:     config.Siacore.HostDirectory,
		WalletFile:  config.Siad.WalletFile,
		ServerAddr:  config.Siacore.RPCaddr,
		Nobootstrap: config.Siacore.NoBootstrap,

		State: d.state,

		Host:   Host,
		HostDB: hostDB,
		Miner:  d.miner,
		Renter: Renter,
		Wallet: d.wallet,
	}

	d.core, err = sia.CreateCore(siaconfig)
	if err != nil {
		return
	}

	// Begin listening for requests on the API.
	d.handle(config.Siad.APIaddr)

	return
}
