package main

import (
	"fmt"
	"math/big"
	"os"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	hostCmd = &cobra.Command{
		Use:   "host",
		Short: "Perform host actions",
		Long:  "View or modify host settings.",
		Run:   wrap(hostcmd),
	}

	hostConfigCmd = &cobra.Command{
		Use:   "config [setting] [value]",
		Short: "Modify host settings",
		Long: `Modify host settings.
Available settings:
	totalstorage
	minduration
	maxduration
	windowsize
	price (in SC per GB per month)
	acceptingcontracts

To configure the host to not accept new contracts, set acceptingcontracts
to false, e.g.:
	siac host config acceptingcontracts false
`,
		Run: wrap(hostconfigcmd),
	}

	hostAnnounceCmd = &cobra.Command{
		Use:   "announce",
		Short: "Announce yourself as a host",
		Long: `Announce yourself as a host on the network.
You may also supply a specific address to be announced, e.g.:
	siac host announce my-host-domain.com:9001
Doing so will override the standard connectivity checks.`,
		Run: hostannouncecmd,
	}
)

// hostconfigcmd is the handler for the command `siac host config [setting] [value]`.
// Modifies host settings.
func hostconfigcmd(param, value string) {
	switch param {
	case "price":
		// convert price to hastings/byte/block
		p, ok := new(big.Rat).SetString(value)
		if !ok {
			die("Could not parse price")
		}
		p.Mul(p, big.NewRat(1e24/1e9, 4320))
		value = new(big.Int).Div(p.Num(), p.Denom()).String()
	case "totalstorage":
		// parse sizes of form 10GB, 10TB, 1TiB etc
		var err error
		value, err = parseSize(value)
		if err != nil {
			die("Could not parse totalstorage:", err)
		}
	case "minduration", "maxduration", "windowsize", "acceptingcontracts": // Other valid settings.
	default:
		// Reject invalid host config commands.
		die("\"" + param + "\" is not a host setting")
	}
	err := post("/host", param+"="+value)
	if err != nil {
		die("Could not update host settings:", err)
	}
	fmt.Println("Host settings updated.")
}

// hostannouncecmd is the handler for the command `siac host announce`.
// Announces yourself as a host to the network. Optionally takes an address to
// announce as.
func hostannouncecmd(cmd *cobra.Command, args []string) {
	var err error
	switch len(args) {
	case 0:
		err = post("/host/announce", "")
	case 1:
		err = post("/host/announce", "netaddress="+args[0])
	default:
		cmd.Usage()
		os.Exit(exitCodeUsage)
	}
	if err != nil {
		die("Could not announce host:", err)
	}
	fmt.Println("Host announcement submitted to network.")
}

// hostcmd is the handler for the command `siac host`.
// Prints info about the host.
func hostcmd() {
	hg := new(api.HostGET)
	err := getAPI("/host", &hg)
	if err != nil {
		die("Could not fetch host settings:", err)
	}
	// convert accepting bool
	accept := "Yes"
	if !hg.AcceptingContracts {
		accept = "No"
	}
	// convert price to SC/GB/mo
	price := new(big.Rat).SetInt(hg.Price.Big())
	price.Mul(price, big.NewRat(4320, 1e24/1e9))
	fmt.Printf(`Host info:
	Storage:      %v (%v used)
	Price:        %v SC per GB per month
	Max Duration: %v Blocks

	Contracts:           %v
	Accepting Contracts: %v
	Anticipated Revenue: %v
	Revenue:             %v
	Lost Revenue:        %v
`, filesizeUnits(hg.TotalStorage), filesizeUnits(hg.TotalStorage-hg.StorageRemaining),
		price.FloatString(3), hg.MaxDuration, hg.NumContracts, accept,
		hg.AnticipatedRevenue, hg.Revenue, hg.LostRevenue)

	// display more info if verbose flag is set
	if !hostVerbose {
		return
	}
	fmt.Printf(`
	Net Address: %v
	Unlock Hash: %v
	(NOT a wallet address!)

RPC Stats:
	Error Calls:        %v
	Unrecognized Calls: %v
	Download Calls:     %v
	Renew Calls:        %v
	Revise Calls:       %v
	Settings Calls:     %v
	Upload Calls:       %v
`, hg.NetAddress, hg.UnlockHash, hg.RPCErrorCalls, hg.RPCUnrecognizedCalls, hg.RPCDownloadCalls,
		hg.RPCRenewCalls, hg.RPCReviseCalls, hg.RPCSettingsCalls, hg.RPCUploadCalls)
}
