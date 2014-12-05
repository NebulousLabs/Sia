package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/siacore"
	"github.com/NebulousLabs/Andromeda/siad"
)

// Pulls a bunch of information and announces the host to the network.
func becomeHostWalkthrough(e *siad.Environment) (err error) {
	// Get a volume of storage to sell.
	fmt.Print("Amount of storage to sell (in MB): ")
	var storage uint64
	_, err = fmt.Scanln(&storage)
	if err != nil {
		return
	}

	// Get a price to sell it at.
	fmt.Print("Price of storage (siacoins per kilobyte): ")
	var price siacore.Currency
	_, err = fmt.Scanln(&price)
	if err != nil {
		return
	}

	// Get a volume of coins to freeze.
	fmt.Print("How many coins to freeze (more is better): ")
	var freezeCoins siacore.Currency
	_, err = fmt.Scanln(&freezeCoins)
	if err != nil {
		return
	}

	// Get a lenght of time to freeze the coins.
	fmt.Print("How many blocks to freeze the coins (more is better): ")
	var freezeBlocks siacore.BlockHeight
	_, err = fmt.Scanln(&freezeBlocks)
	if err != nil {
		return
	}

	// Create the host announcement structure.
	hostSettings := siad.HostAnnouncement{
		IPAddress:             e.NetAddress(),
		MinFilesize:           1024 * 1024, // 1mb
		MaxFilesize:           storage * 1024 * 1024,
		MinDuration:           2000,
		MaxDuration:           10000,
		MinChallengeFrequency: 250,
		MaxChallengeFrequency: 100,
		MinTolerance:          10,
		Price:                 price,
		Burn:                  price,
		CoinAddress:           e.CoinAddress(),
		// SpendConditions and FreezeIndex handled by HostAnnounceSelf
	}
	e.SetHostSettings(hostSettings)

	// Have the wallet make the announcement.
	_, err = e.HostAnnounceSelf(freezeCoins, freezeBlocks+e.Height(), 10)
	if err != nil {
		return
	}

	return
}
