package main

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Andromeda/siacore"
	"github.com/NebulousLabs/Andromeda/siad"
)

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
	e.ToggleMining(e.state, e.wallet.SpendConditions.CoinAddress())
}

// printWalletAddresses prints out all of the addresses that are spendable by
// this cli.
func printWalletAddresses(e *siad.Environment) {
	fmt.Println("General Information:")

	// Dispaly whether or not the miner is mining.
	if e.miner.Mining() {
		fmt.Println("\tMining Status: ON - Wallet is mining on one thread.")
	} else {
		fmt.Println("\tMining Status: OFF - Wallet is not mining.")
	}
	fmt.Println()

	fmt.Printf("\tWallet Address: %x\n", e.wallet.SpendConditions.CoinAddress())
	fmt.Println()

	fmt.Println("\tCurrent Block Height:", e.state.Height())
	fmt.Println("\tCurrent Block Target:", e.state.CurrentTarget())
	fmt.Println("\tCurrent Block Depth:", e.state.Depth())
	fmt.Println()

	fmt.Println("\tPrinting Networked Peers:")
	addresses := e.server.AddressBook()
	for _, address := range addresses {
		fmt.Printf("\t\t%v:%v\n", address.Host, address.Port)
	}
	fmt.Println()
}

// sendCoinsWalkthorugh uses the wallets in the environment to send coins to an
// address that is provided through the command line.
func sendCoinsWalkthrough(e *siad.Environment) (err error) {
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

	// Convert the address to a siacore.CoinAddress
	var address siacore.CoinAddress
	copy(address[:], addressBytes)

	// Use the wallet api to send.
	fmt.Printf("Sending %v coins with miner fee of %v to address %x", amount, minerFee, address[:])
	_, err = e.wallet.SpendCoins(siacore.Currency(amount), siacore.Currency(minerFee), address)
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
		"c:\tCatch Up - collect blocks you are missing.\n",
		"H:\tHost - become a host and announce to the network\n",
		"m:\tMine - turn mining on or off\n",
		"p\tPrint - list all of the wallets, plus some stats about the program\n",
		"s:\tSend - send coins to another wallet\n",
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
				// Need a lock here.
				e.caughtUp = false
				err := e.state.CatchUp(e.server.RandomPeer())
				if err != nil {
					fmt.Println("CatchUp Error:", err)
				}
				e.caughtUp = true
			}()

		case "H", "host", "store", "advertise", "storage":
			err = becomeHostWalkthrough(e)

		case "m", "mine", "toggle", "mining":
			if !e.caughtUp {
				err = errors.New("not caught up to peers yet. Please wait.")
			} else {
				toggleMining(e)
			}

		case "p", "print":
			printWalletAddresses(e)

		/*
			case "r", "rent":
				becomeClientWalkthrough(e)
		*/

		/*
			case "S", "save":
				saveWalletEnvironmentWalkthorugh()
		*/

		case "s", "send":
			err = sendCoinsWalkthrough(e)
		}

		if err != nil {
			fmt.Println("Error:", err)
			err = nil
		}
	}
}
