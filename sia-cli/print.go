package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/siad"
)

// printWalletAddresses prints out all of the addresses that are spendable by
// this cli.
func printEnvironmentInfo(e *siad.Environment) {
	fmt.Println("General Information:")

	// Dispaly whether or not the miner is mining.
	if e.Mining() {
		fmt.Println("\tMining Status: ON - Wallet is mining on one thread.")
	} else {
		fmt.Println("\tMining Status: OFF - Wallet is not mining.")
	}
	fmt.Println()

	fmt.Printf("\tWallet Address: %x\n", e.CoinAddress())
	fmt.Printf("\tWallet Balance: %v\n", e.WalletBalance())
	fmt.Println()

	info := e.StateInfo()
	fmt.Println("\tCurrent Block Height:", info.Height)
	fmt.Println("\tCurrent Block Target:", info.Target)
	fmt.Println("\tCurrent Block Depth:", info.Depth)
	fmt.Println()

	fmt.Println("\tPrinting Networked Peers:")
	addresses := e.AddressBook()
	for _, address := range addresses {
		fmt.Printf("\t\t%v:%v\n", address.Host, address.Port)
	}
	fmt.Println()

	fmt.Println("\tPrinting friend list:")
	friends := e.FriendMap()
	for name, address := range friends {
		fmt.Printf("\t\t%v\t%x\n", name, address)
	}
	fmt.Println()
}

// printDeep eventually intends to print out essentially everything that would
// be hashed in StateHash.
func printDeep(e *siad.Environment) {
	info := e.StateInfo()

	fmt.Println("Deep Printing:")
	fmt.Println()
	fmt.Println("\tState Hash:", info.StateHash)
	fmt.Println()
	fmt.Println("\t\tHeight:", info.Height)
	fmt.Println("\t\tTarget:", info.Target)
	fmt.Println("\t\tDepth:", info.Depth)
	fmt.Println("\t\tEarliest Legal Timestamp:", info.EarliestLegalTimestamp)
	fmt.Println()
	fmt.Println("\tUtxo Set:", len(info.UtxoSet))
	for _, utxo := range info.UtxoSet {
		output, err := e.Output(utxo)
		if err != nil {
			fmt.Println("Error during deep print:", err)
			continue
		}

		fmt.Println("\t\tHash:", utxo)
		fmt.Println("\t\tValue:", output.Value)
		fmt.Println("\t\tSpendHash:", output.SpendHash)
		fmt.Printf("\t\tCoinAddress: %x\n", output.SpendHash)
		fmt.Println()
	}
}
