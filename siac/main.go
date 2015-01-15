package main

import (
	"os"

	"github.com/spf13/cobra"
)

func versioncmd(*cobra.Command, []string) {
	println("Sia Client v0.1.0")
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Client v0.1.0",
		Long:  "Sia Client v0.1.0",
		Run:   versioncmd,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information.",
		Run:   versioncmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the Sia daemon",
		Long:  "Stop the Sia daemon.",
		Run:   stopcmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "mine [on|off]",
		Short: "Start or stop mining",
		Long:  "Start or stop mining blocks.",
		Run:   minecmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Synchronize with the network",
		Long:  "Attempt to synchronize with a randomly selected peer.",
		Run:   synccmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "peer [add|remove] [address]",
		Short: "Manually add or remove a peer",
		Long:  "Manually add or remove a peer from the server's peer list.",
		Run:   peercmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "send [amount] [fee] [dest]",
		Short: "Send coins to an address",
		Long:  "Send coins to an address, or to a friend. The destination is first interpreted as an friend, and then as an address if the friend lookup fails.",
		Run:   sendcmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "host [MB] [price] [freezecoins] [freezeblocks]",
		Short: "Become a host",
		Long:  "Submit a host announcement to the network, including the amount of storage offered and the price of renting storage.",
		Run:   hostcmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "rent [filename] [nickname]",
		Short: "Store a file on a host",
		Long:  "Negotiate a file contract with a host, and upload the file to them if negotiation is successful.",
		Run:   rentcmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "download [nickname] [destination]",
		Short: "Download a file from a host",
		Long:  "Download a file previously stored with a host",
		Run:   downloadcmd,
	})

	status := &cobra.Command{
		Use:   "status [check|apply]",
		Short: "Print the current state of the daemon",
		Long:  "Query the daemon for values such as the current difficulty, target, height, peers, transactions, etc.",
		Run:   statuscmd,
	}
	root.AddCommand(status)

	update := &cobra.Command{
		Use:   "update",
		Short: "Update Sia",
		Long:  "Check for (and/or download) available updates for Sia.",
		Run:   updatecmd,
	}
	root.AddCommand(update)

	root.Execute()
}
