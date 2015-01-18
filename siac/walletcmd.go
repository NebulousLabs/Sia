package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	walletCmd = &cobra.Command{
		Use:   "wallet [address|send|status]",
		Short: "Perform wallet actions",
		Long:  "Generate a new address, send coins to another wallet, or view info about the wallet.",
		Run:   wrap(walletstatuscmd),
	}

	walletAddressCmd = &cobra.Command{
		Use:   "wallet address",
		Short: "Get a new wallet address",
		Long:  "Generate a new wallet address.",
		Run:   wrap(walletaddresscmd),
	}

	walletSendCmd = &cobra.Command{
		Use:   "wallet send [amount] [dest]",
		Short: "Send coins to another wallet",
		Long:  "Send coins to another wallet. 'dest' must be a 64-byte hexadecimal address.",
		Run:   wrap(walletsendcmd),
	}

	walletStatusCmd = &cobra.Command{
		Use:   "wallet status",
		Short: "View wallet status",
		Long:  "View wallet status, including the current balance and number of addresses.",
		Run:   wrap(walletstatuscmd),
	}
)

func walletaddresscmd() {
	addr, err := getWalletAddress()
	if err != nil {
		fmt.Println("Could not generate new address:", err)
		return
	}
	fmt.Println("Created new address:", addr)
}

func walletsendcmd(amount, dest string) {
	err := getWalletSend(amount, dest)
	if err != nil {
		fmt.Println("Could not send:", err)
		return
	}
	fmt.Println("Sent", amount, "coins to", dest)
}

func walletstatuscmd() {
	status, err := getWalletStatus()
	if err != nil {
		fmt.Println("Could not get wallet status:", err)
		return
	}
	fmt.Println(m)
}
