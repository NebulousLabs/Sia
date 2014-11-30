package main

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Andromeda/siacore"
	"github.com/NebulousLabs/Andromeda/siad"
)

// Read in the name of a friend and get the corresponding address from the
// friend map.
func readFriendAddress(e *siad.Environment) (address siacore.CoinAddress, err error) {
	var friendName string
	fmt.Print("Friend to send coins to: ")
	_, err = fmt.Scanln(&friendName)
	if err != nil {
		return
	}

	friendMap := e.FriendMap()
	address, exists := friendMap[friendName]
	if !exists {
		err = errors.New("could not find friend")
		return
	}

	return
}

// Read in a CoinAddress in hex and return the corresponding CoinAddress.
func readCoinAddress() (address siacore.CoinAddress, err error) {
	var addressBytes []byte
	fmt.Print("Address of Receiving Wallet: ")
	_, err = fmt.Scanf("%x", &addressBytes)
	if err != nil {
		return
	}

	// Convert the address to a siacore.CoinAddress
	copy(address[:], addressBytes)

	return
}

// sendCoinsWalkthorugh uses the wallets in the environment to send coins to an
// address that is provided through the command line.
func sendCoinsWalkthrough(e *siad.Environment) (err error) {
	fmt.Println("Send Coins Walkthrough:")
	fmt.Println()

	fmt.Print("Amount to send: ")
	var amount siacore.Currency
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

	// Determine whether to send to a friend or manual address.
	fmt.Print("Send to a friend (f) or address (a): ")
	var sendType string
	_, err = fmt.Scanln(&sendType)
	if err != nil {
		return
	}

	// Get the address using the desired method.
	var address siacore.CoinAddress
	if sendType == "f" {
		address, err = readFriendAddress(e)
	} else if sendType == "a" {
		address, err = readCoinAddress()
		if err != nil {
			return
		}
	} else {
		err = errors.New("Did not understand response")
		return
	}

	// Use the wallet api to send.
	fmt.Printf("Sending %v coins with miner fee of %v to address %x", amount, minerFee, address[:])
	_, err = e.SpendCoins(amount, siacore.Currency(minerFee), address)
	if err != nil {
		return
	}
	fmt.Println()

	return
}
