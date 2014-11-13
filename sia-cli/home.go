package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/sia"
)

// Pulls a bunch of information and announces the host to the network.
func becomeHostWalkthrough(env *walletEnvironment) (err error) {
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

	// NEED TO GET IP ADDRESS SOMEWHERE.

	// Create the host announcement structure.
	ha := sia.HostAnnouncement{
		// IPAddress: "asdf",
		MinFilesize:           1024 * 1024, // 1mb
		MaxFilesize:           storage * 1024 * 1024,
		MaxDuration:           10000,
		MaxChallengeFrequency: sia.BlockHeight(100),
		MinTolerance:          10,
		Price:                 sia.Currency(price),
		Burn:                  sia.Currency(price),
		CoinAddress:           env.wallets[0].SpendConditions.CoinAddress(),
	}

	// Have the wallet make the announcement.
	_, err = env.wallets[0].HostAnnounceSelf(ha, sia.Currency(freezeCoins), sia.BlockHeight(freezeBlocks)+env.state.Height(), 0, env.state)
	if err != nil {
		return
	}

	return
}

func toggleMining(env *walletEnvironment) {
	go env.state.ToggleMining(env.wallets[0].SpendConditions.CoinAddress())
}

// printWalletAddresses prints out all of the addresses that are spendable by
// this cli.
func printWalletAddresses(env *walletEnvironment) {
	fmt.Println("General Information:")

	// Dispaly whether or not the miner is mining.
	if env.state.Mining {
		fmt.Println("\tMining Status: ON - Wallet is mining on one thread.")
	} else {
		fmt.Println("\tMining Status: OFF - Wallet is not mining.")
	}
	fmt.Println()

	fmt.Println("\tCurrent Block Height:", env.state.Height())
	fmt.Println("\tCurrent Block Depth:", env.state.Depth())
	fmt.Println()

	fmt.Println("\tPrinting all valid wallet addresses.")
	for _, wallet := range env.wallets {
		fmt.Printf("\t\t%x\n", wallet.SpendConditions.CoinAddress())
	}
}

// sendCoinsWalkthorugh uses the wallets in the environment to send coins to an
// address that is provided through the command line.
func sendCoinsWalkthrough(env *walletEnvironment) (err error) {
	fmt.Println("Send Coins Walkthrough:")

	fmt.Print("Amount to send: ")
	var amount int
	_, err = fmt.Scanln(&amount)
	if err != nil {
		return
	}

	fmt.Print("Amount to use as Miner Fee: ")
	var minerFee int
	_, err = fmt.Scanln(&minerFee)
	if err != nil {
		return
	}

	fmt.Print("Address of Receiving Wallet: ")
	var addressBytes []byte
	_, err = fmt.Scanf("%x", &addressBytes)
	if err != nil {
		return
	}

	// Convert the address to a sia.CoinAddress
	var address sia.CoinAddress
	copy(address[:], addressBytes)

	// Use the wallet api to send. ==> Only uses wallets[0] for the time being.
	fmt.Printf("Sending %v coins with miner fee of %v to address %x", amount, minerFee, address[:])
	_, err = env.wallets[0].SpendCoins(sia.Currency(amount), sia.Currency(minerFee), address, env.state)
	if err != nil {
		return
	}

	return
}

// displayHomeHelp lists all of the options available at the home screen, with
// descriptions.
func displayHomeHelp() {
	fmt.Println(
		" h:\tHelp - display this message\n",
		"q:\tQuit - quit the program\n",
		"H:\tHost - become a host and announce to the network\n",
		"m:\tMine - turn mining on or off\n",
		"p\tPrint - list all of the wallets, plus some stats about the program\n",
		"s:\tSend - send coins to another wallet\n",
	)
}

// pollHome repeatedly querys the user for input.
func pollHome(env *walletEnvironment) {
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

		case "H", "host", "store", "advertise", "storage":
			err = becomeHostWalkthrough(env)

		case "m", "mine", "toggle", "mining":
			toggleMining(env)

		case "p", "print":
			printWalletAddresses(env)

		/*
			case "r", "rent":
				becomeClientWalkthrough(env)
		*/

		/*
			case "S", "save":
				saveWalletEnvironmentWalkthorugh()
		*/

		case "s", "send":
			err = sendCoinsWalkthrough(env)
		}

		if err != nil {
			fmt.Println("Error:", err)
			err = nil
		}
	}
}
