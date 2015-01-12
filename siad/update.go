package main

import (
	_ "github.com/inconshreveable/go-update"
)

func (d *daemon) checkForUpdate() bool {
	return false
}

func (d *daemon) applyUpdate() bool {
	return true
}
