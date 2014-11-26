package main

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Andromeda/siad"
)

// saveCoinAddressWalkthrough steps the user through saving their environment
// coin address.
func loadCoinAddressWalkthrough(e *siad.Environment) (err error) {
	// Get filename.
	fmt.Print("Filename for the coin address: ")
	var filename string
	_, err = fmt.Scanln(&filename)
	if err != nil {
		return
	}
	fmt.Println()

	// Get friend name.
	var friendName string
	fmt.Print("Id/name for the coin address: ")
	_, err = fmt.Scanln(&friendName)
	if err != nil {
		return
	}
	fmt.Println()

	// Call the environment function to handle the rest.
	err = e.LoadCoinAddress(filename, friendName)
	if err != nil {
		return
	}

	return
}

// saveWalkthough present options for the various things that can be saved, and
// saves the item chosen by the user.
func loadWalkthrough(e *siad.Environment) (err error) {
	fmt.Println("Load Walkthrough - What would you like to load?")
	fmt.Println("\tc - a coin address")
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
		err = loadCoinAddressWalkthrough(e)
		return
	}
}
