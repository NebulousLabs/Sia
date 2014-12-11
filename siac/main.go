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
		Use:   "mine",
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
		Use:   "send",
		Short: "Send coins to an address",
		Long:  "Send coins to an address, or to a friend. The destination is first interpreted as an friend, and then as an address if the friend lookup fails.",
		Run:   sendcmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "host",
		Short: "Become a host",
		Long:  "Submit a host announcement to the network, including the amount of storage offered and the price of renting storage.",
		Run:   hostcmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "rent",
		Short: "Store a file on a host",
		Long:  "Negotiate a file contract with a host, and upload the file to them if negotiation is successful.",
		Run:   rentcmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "download",
		Short: "Download a file from a host",
		Long:  "Download a file previously stored with a host",
		Run:   downloadcmd,
	})

	save := &cobra.Command{
		Use:   "save",
		Short: "Save the wallet address of the server",
		Long:  "Save the wallet address of the server to a specified file.",
		Run:   savecmd,
	}
	root.AddCommand(save)

	load := &cobra.Command{
		Use:   "load",
		Short: "Load a wallet address",
		Long:  "Load the wallet address of another peer.",
		Run:   loadcmd,
	}
	root.AddCommand(load)

	status := &cobra.Command{
		Use:   "status",
		Short: "Print the current state of the daemon",
		Long:  "Query the daemon for values such as the current difficulty, target, height, peers, transactions, etc.",
		Run:   statuscmd,
	}
	root.AddCommand(status)

	root.Execute()
}
