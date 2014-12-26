package main

import (
	"fmt"
	"html/template"
	"os"

	"github.com/NebulousLabs/Sia/sia"

	"github.com/mitchellh/go-homedir"
)

type daemon struct {
	core *sia.Core

	styleDir    string
	downloadDir string

	template *template.Template
}

func createDaemon(config Config) (d *daemon, err error) {
	// Expand any '~' characters in the config directories.
	expandedHostDir, err := homedir.Expand(config.Siacore.HostDirectory)
	if err != nil {
		err = fmt.Errorf("problem with hostDir: %v", err)
		return
	}
	expandedStyleDir, err := homedir.Expand(config.Siad.StyleDirectory)
	if err != nil {
		err = fmt.Errorf("problem with styleDir: %v", err)
		return
	}
	expandedDownloadDir, err := homedir.Expand(config.Siad.DownloadDirectory)
	if err != nil {
		err = fmt.Errorf("problem with downloadDir: %v", err)
		return
	}
	expandedWalletFile, err := homedir.Expand(config.Siad.WalletFile)
	if err != nil {
		err = fmt.Errorf("problem with walletFile: %v", err)
		return
	}

	// Create downloads directory and host directory.
	err = os.MkdirAll(expandedDownloadDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return
	}
	err = os.MkdirAll(expandedHostDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return
	}

	// Create and fill out the daemon object.
	d = &daemon{
		styleDir:    expandedStyleDir,
		downloadDir: expandedDownloadDir,
	}

	// Create the web interface template.
	d.template, err = template.ParseFiles(expandedStyleDir + "template.html")
	if err != nil {
		err = fmt.Errorf("template.html not found! Please move the styles folder to '%v'", expandedStyleDir)
		return
	}

	d.core, err = sia.CreateCore(expandedHostDir, expandedWalletFile, config.Siacore.RPCaddr, config.Siacore.NoBootstrap)
	if err != nil {
		return
	}

	// Begin listening for requests on the api.
	d.setUpHandlers(config.Siad.APIaddr)

	return
}
