package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/siad"
)

func becomeRenterWalkthrough(e *siad.Environment) (err error) {
	// Get filename to download
	fmt.Print("Filename: ")
	var filename string
	_, err = fmt.Scanln(&filename)
	if err != nil {
		return
	}

	err = e.ClientProposeContract(filename)
	return
}

func downloadWalkthrough(e *siad.Environment) (err error) {
	// Get filename to download
	fmt.Print("Filename: ")
	var filename string
	_, err = fmt.Scanln(&filename)
	if err != nil {
		return
	}

	err = e.Download(filename)
	return
}
