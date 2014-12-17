package main

import (
	"fmt"
	"html/template"
	"os"

	"github.com/NebulousLabs/Sia/siacore"

	"github.com/mitchellh/go-homedir"
)

type daemon struct {
	core *siacore.Environment

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

	// Create downloads directory and host directory.
	err = os.MkdirAll(expandedDownloadDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return
	}
	err = os.MkdirAll(expandedHostDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return
	}

	// Check that template.html exists.
	if _, err = os.Stat(expandedStyleDir + "template.html"); err != nil {
		err = fmt.Errorf("template.html not found! Please put the styles/ folder into '%v'", expandedStyleDir)
		return
	}

	// Create and fill out the daemon object.
	d = &daemon{
		styleDir:    expandedStyleDir,
		downloadDir: expandedDownloadDir,
	}
	d.core, err = siacore.CreateEnvironment(expandedHostDir, config.Siacore.RpcPort, config.Siacore.NoBootstrap)
	if err != nil {
		return
	}
	// Create the web interface template.
	d.template = template.Must(template.ParseFiles(expandedStyleDir + "template.html"))

	// Begin listening for requests on the api.
	d.setUpHandlers(config.Siad.ApiPort)

	return
}
