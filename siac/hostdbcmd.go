package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	hostdbCmd = &cobra.Command{
		Use:   "hostdb",
		Short: "View or modify the host database",
		Long:  "Add and remove hosts, or list active hosts on the network.",
		Run:   wrap(hostdbcmd),
	}
)

func hostdbcmd() {
	info := new(api.HostdbActiveGET)
	err := getAPI("/hostdb/active", info)
	if err != nil {
		die("Could not fetch host list:", err)
	}
	if len(info.Hosts) == 0 {
		fmt.Println("No known active hosts")
		return
	}
	fmt.Println(len(info.Hosts), "active hosts:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Address\tPrice")
	for _, host := range info.Hosts {
		price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
		fmt.Fprintf(w, "%v\t%v / TB / Month\n", host.NetAddress, currencyUnits(price))
	}
	w.Flush()
}
