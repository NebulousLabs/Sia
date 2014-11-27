package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/siad"
)

/*
// Pulls a bunch of information and announces the host to the network.
func becomeHostWalkthrough(e *siad.Environment) (err error) {
	// Get a volume of days to freeze the coins.
	// Burn will be equal to price.
	// Frequency will be 100.

	// Get a volume of storage to sell.
	fmt.Print("Amount of storage to sell (in MB): ")
	var storage uint64
	_, err = fmt.Scanln(&storage)
	if err != nil {
		return
	}

	// Get a price to sell it at.
	fmt.Print("Price of storage (siacoins per kilobyte): ")
	var price uint64
	_, err = fmt.Scanln(&price)
	if err != nil {
		return
	}

	// Get a volume of coins to freeze.
	fmt.Print("How many coins to freeze (more is better): ")
	var freezeCoins uint64
	_, err = fmt.Scanln(&freezeCoins)
	if err != nil {
		return
	}

	fmt.Print("How many blocks to freeze the coins (more is better): ")
	var freezeBlocks uint64
	_, err = fmt.Scanln(&freezeBlocks)
	if err != nil {
		return
	}

	// Create the host announcement structure.
	e.wallet.HostSettings = siad.HostAnnouncement{
		IPAddress:             e.server.NetAddress(),
		MinFilesize:           1024 * 1024, // 1mb
		MaxFilesize:           storage * 1024 * 1024,
		MaxDuration:           10000,
		MaxChallengeFrequency: siacore.BlockHeight(100),
		MinTolerance:          10,
		Price:                 siacore.Currency(price),
		Burn:                  siacore.Currency(price),
		CoinAddress:           e.wallet.SpendConditions.CoinAddress(),
		// SpendConditions and FreezeIndex handled by HostAnnounceSelg
	}

	// Have the wallet make the announcement.
	_, err = e.wallet.HostAnnounceSelf(siacore.Currency(freezeCoins), siacore.BlockHeight(freezeBlocks)+e.state.Height(), 0, e.state)
	if err != nil {
		return
	}

	return
}
*/

// toggleMining asks the state to switch mining on or off.
func toggleMining(e *siad.Environment) {
	e.ToggleMining()
}

// displayHomeHelp lists all of the options available at the home screen, with
// descriptions.
func displayHomeHelp() {
	fmt.Println(
		" h:\tHelp - display this message\n",
		"q:\tQuit - quit the program\n",
		"c:\tCatch Up - collect blocks you are missing.\n",
		// "H:\tHost - become a host and announce to the network\n",
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

		/*
			case "H", "host", "store", "advertise", "storage":
				err = becomeHostWalkthrough(e)
		*/

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
