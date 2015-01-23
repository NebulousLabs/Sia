package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules/wallet"
)

var (
	walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "Perform wallet actions",
		Long:  "Generate a new address, send coins to another wallet, or view info about the wallet.",
		Run:   wrap(walletstatuscmd),
	}

	walletAddressCmd = &cobra.Command{
		Use:   "address",
		Short: "Get a new wallet address",
		Long:  "Generate a new wallet address.",
		Run:   wrap(walletaddresscmd),
	}

	walletSendCmd = &cobra.Command{
		Use:   "send [amount] [dest]",
		Short: "Send coins to another wallet",
		Long:  "Send coins to another wallet. 'dest' must be a 64-byte hexadecimal address.",
		Run:   wrap(walletsendcmd),
	}

	walletStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View wallet status",
		Long:  "View wallet status, including the current balance and number of addresses.",
		Run:   wrap(walletstatuscmd),
	}
)

// TODO: this should be defined outside of siac
type walletAddr struct {
	Address string
}

func walletaddresscmd() {
	addr := new(walletAddr)
	err := getAPI("/wallet/address", addr)
	if err != nil {
		fmt.Println("Could not generate new address:", err)
		return
	}
	fmt.Printf("Created new address: %s\n", addr.Address)
}

func walletsendcmd(amount, dest string) {
	err := callAPI(fmt.Sprintf("/wallet/send?amount=%s&dest=%s", amount, dest))
	if err != nil {
		fmt.Println("Could not send:", err)
		return
	}
	fmt.Printf("Sent %s coins to %s\n", amount, dest)
}

func walletstatuscmd() {
	status := new(wallet.WalletInfo)
	err := getAPI("/wallet/status", status)
	if err != nil {
		fmt.Println("Could not get wallet status:", err)
		return
	}
	fmt.Printf(`Wallet status:
Balance:   %v (confirmed) 
           %v (unconfirmed)
Addresses: %d
`, status.Balance, status.FullBalance, status.NumAddresses)
}
