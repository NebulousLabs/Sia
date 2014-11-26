package main

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Andromeda/siad"
)

func saveCoinAddressWalkthrough(e *siad.Environment) (err error) {
	fmt.Print("Filename for your coin address: ")
	var filename string
	_, err = fmt.Scanln(&filename)
	if err != nil {
		return
	}
	fmt.Println()

	err = e.SaveCoinAddress(filename)
	if err != nil {
		return
	}

	return
}

// saveWalkthough present options for the various things that can be saved, and
// saves the item chosen by the user.
func saveWalkthrough(e *siad.Environment) (err error) {
	fmt.Println("Save Walkthrough - What would you like to save?")
	fmt.Println("\tc - CoinAddress")
	fmt.Println()

	fmt.Print("Your Choice: ")
	var option string
	_, err = fmt.Scanln(&option)
	if err != nil {
		return
	}
	fmt.Println()

	switch option {
	default:
		err = errors.New("could not understand input.")
		return

	case "c", "ca", "coin address", "CoinAddress":
		err = saveCoinAddressWalkthrough(e)
		return
	}
}
