package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/types"
)

var (
	hostdbCmd = &cobra.Command{
		Use:   "hostdb",
		Short: "View or modify the host database",
		Long:  "Add and remove hosts, or list active hosts on the network.",
		Run:   wrap(hostdblistcmd),
	}

	// DEPRECATED v0.5.2
	hostdbDeprecatedCmd = &cobra.Command{
		Use:        "hostdb",
		Deprecated: "use `siac hostdb` instead.",
		Run:        wrap(hostdblistcmd),
	}

	hostdbListCmd = &cobra.Command{
		Use:   "list",
		Short: "List active hosts on the network",
		Long:  "List active hosts on the network.",
		Run:   wrap(hostdblistcmd),
	}
)

func hostdblistcmd() {
	info := new(api.ActiveHosts)
	err := getAPI("/renter/hosts/active", info)
	if err != nil {
		die("Could not fetch host list:", err)
	}
	if len(info.Hosts) == 0 {
		fmt.Println("No known active hosts")
		return
	}
	fmt.Println("Active hosts:")
	for _, host := range info.Hosts {
		fmt.Printf("\t%v - %v SC / GB / Mo\n", host.NetAddress, host.ContractPrice.Mul(types.NewCurrency64(4320e9)).Div(types.SiacoinPrecision))
	}
}
