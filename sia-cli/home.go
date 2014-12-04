package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/siad"
)

// displayHomeHelp lists all of the options available at the home screen, with
// descriptions.
func displayHomeHelp() {
	fmt.Println(
		" h:\tHelp - display this message\n",
		"q:\tQuit - quit the program\n",
		"c:\tCatch Up - collect blocks you are missing.\n",
		"H:\tHost - become a host and announce to the network\n",
		"L:\tLoad - load a secret key or coin address.\n",
		"m:\tMine - turn mining on or off\n",
		"p\tPrint - list all of the wallets, plus some stats about the program.\n",
		"s:\tSend - send coins to another wallet\n",
		"S:\tSave - save your secret key or coin address\n",
	)
}

// pollHome repeatedly querys the user for input.
func pollHome(e *siad.Environment) {
	var input string
	var err error
	for {
		fmt.Println()
		fmt.Print("(Home) Please enter a command: ")
		_, err = fmt.Scanln(&input)
		if err != nil {
			continue
		}
		fmt.Println()

		switch input {
		default:
			fmt.Println("unrecognized command")

		case "?", "h", "help":
			displayHomeHelp()

		case "q", "quit":
			return

		case "c", "catch":
			// Dirty that the error just inserts itself into whatever the user
			// is doing.
			go func() {
				err := e.CatchUp(e.RandomPeer())
				if err != nil {
					fmt.Println("CatchUp Error:", err)
				}
			}()

		case "H", "host", "store", "advertise", "storage":
			err = becomeHostWalkthrough(e)

		case "L", "load":
			err = loadWalkthrough(e)

		case "m", "mine", "toggle", "mining":
			err = e.ToggleMining()

		case "p", "print":
			printEnvironmentInfo(e)

		case "P":
			printDeep(e)

		/*
			case "r", "rent":
				becomeClientWalkthrough(e)
		*/

		case "S", "save":
			saveWalkthrough(e)

		case "s", "send":
			err = sendCoinsWalkthrough(e)
		}

		if err != nil {
			fmt.Println("Error:", err)
			err = nil
		}
	}
}
