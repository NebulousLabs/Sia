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

	state, _ := consensus.CreateGenesisState() // the `_` is not of type error.
	Wallet, err := wallet.New(state, config.Siad.WalletFile)
	if err != nil {
		return
	}
	hostDB, err := hostdb.New()
	if err != nil {
		return errors.New("could not load wallet file: " + err.Error())
	}
	Host, err := host.New(state, Wallet)
	if err != nil {
		return
	}
	Renter, err := renter.New(state, hostDB, Wallet)
	if err != nil {
		return
	}

	siaconfig := sia.Config{
		HostDir:     config.Siacore.HostDirectory,
		WalletFile:  config.Siad.WalletFile,
		ServerAddr:  config.Siacore.RPCaddr,
		Nobootstrap: config.Siacore.NoBootstrap,

		State: state,

		Host:   Host,
		HostDB: hostDB,
		Miner:  miner.New(),
		Renter: Renter,
		Wallet: Wallet,
	}

	d.core, err = sia.CreateCore(siaconfig)
	if err != nil {
		return
	}

	// Begin listening for requests on the API.
	d.handle(config.Siad.APIaddr)

	return
}
