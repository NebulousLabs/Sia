package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	hostdbCmd = &cobra.Command{
		Use:   "hostdb",
		Short: "Interact with the renter's host database.",
		Long:  "View the list of active hosts, the list of all hosts, or query specific hosts.",
		Run:   wrap(hostdbcmd),
	}

	hostdbAllCmd = &cobra.Command{
		Use:   "all",
		Short: "View the full set of hosts known to the hostdb.",
		Long:  "View the full set of hosts known to the hostdb.",
		Run:   wrap(hostdballcmd),
	}

	hostdbViewCmd = &cobra.Command{
		Use:   "view [pubkey]",
		Short: "View the full information for a host.",
		Long:  "View detailed information about a host, including things like a score breakdown.",
		Run:   wrap(hostdbviewcmd),
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
	fmt.Println(len(info.Hosts), "Active Hosts:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tAddress\tPrice\tPubkey")
	for _, host := range info.Hosts {
		price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
		fmt.Fprintf(w, "\t%v\t%v / TB / Month\t%v\n", host.NetAddress, currencyUnits(price), host.PublicKeyString)
	}
	w.Flush()
}

func hostdballcmd() {
	info := new(api.HostdbAllGET)
	err := getAPI("/hostdb/all", info)
	if err != nil {
		die("Could not fetch host list:", err)
	}
	if len(info.Hosts) == 0 {
		fmt.Println("No known hosts")
		return
	}
	fmt.Println(len(info.Hosts), "Hosts Total:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tAddress\tPrice\tPubkey")
	for _, host := range info.Hosts {
		price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
		fmt.Fprintf(w, "\t%v\t%v / TB / Month\t%v\n", host.NetAddress, currencyUnits(price), host.PublicKeyString)
	}
	w.Flush()
}

func hostdbviewcmd(pubkey string) {
	info := new(api.HostdbHostsGET)
	err := getAPI("/hostdb/hosts/"+pubkey, info)
	if err != nil {
		die("Could not fetch provided host:", err)
	}

	fmt.Println("Host information:")

	fmt.Println("  Public Key:", info.Entry.PublicKeyString)
	fmt.Println("  Block First Seen:", info.Entry.FirstSeen)

	fmt.Println("\n  Host Settings:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\t\tAccepting Contracts:\t", info.Entry.AcceptingContracts)
	fmt.Fprintln(w, "\t\tTotal Storage:\t", info.Entry.TotalStorage/1e9, "GB")
	fmt.Fprintln(w, "\t\tRemaining Storage:\t", info.Entry.RemainingStorage/1e9, "GB")
	fmt.Fprintln(w, "\t\tOffered Collateral (TB / Mo):\t", info.Entry.Collateral.Mul(modules.BlockBytesPerMonthTerabyte).Div(types.SiacoinPrecision), "SC")
	fmt.Fprintln(w, "\n\t\tContract Price:\t", info.Entry.ContractPrice.Div(types.SiacoinPrecision), "SC")
	fmt.Fprintln(w, "\t\tStorage Price (TB / Mo):\t", info.Entry.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte).Div(types.SiacoinPrecision), "SC")
	fmt.Fprintln(w, "\t\tDownload Price (1 TB):\t", info.Entry.DownloadBandwidthPrice.Mul(modules.BytesPerTerabyte).Div(types.SiacoinPrecision), "SC")
	fmt.Fprintln(w, "\t\tUpload Price (1 TB):\t", info.Entry.UploadBandwidthPrice.Mul(modules.BytesPerTerabyte).Div(types.SiacoinPrecision), "SC")
	fmt.Fprintln(w, "\t\tVersion:\t", info.Entry.Version)
	w.Flush()

	fmt.Println("\n  Score Breakdown:")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	total := info.ScoreBreakdown.AgeAdjustment * info.ScoreBreakdown.BurnAdjustment * info.ScoreBreakdown.CollateralAdjustment * info.ScoreBreakdown.PriceAdjustment * info.ScoreBreakdown.StorageRemainingAdjustment * info.ScoreBreakdown.UptimeAdjustment * info.ScoreBreakdown.VersionAdjustment * 1e9
	fmt.Fprintf(w, "\t\tTotal Score:\t %.0f\n", total)
	fmt.Fprintf(w, "\t\tAge:\t %.3f\n", info.ScoreBreakdown.AgeAdjustment)
	fmt.Fprintf(w, "\t\tBurn:\t %.3f\n", info.ScoreBreakdown.BurnAdjustment)
	fmt.Fprintf(w, "\t\tCollateral:\t %.3f\n", info.ScoreBreakdown.CollateralAdjustment)
	fmt.Fprintf(w, "\t\tPrice:\t %.3f\n", info.ScoreBreakdown.PriceAdjustment*1e3)
	fmt.Fprintf(w, "\t\tStorage:\t %.3f\n", info.ScoreBreakdown.StorageRemainingAdjustment)
	fmt.Fprintf(w, "\t\tUptime:\t %.3f\n", info.ScoreBreakdown.UptimeAdjustment)
	fmt.Fprintf(w, "\t\tVersion:\t %.3f\n", info.ScoreBreakdown.VersionAdjustment)
	w.Flush()

	// Compute the total measured uptime and total measured downtime for this
	// host.
	uptimeRatio := float64(0)
	if len(info.Entry.ScanHistory) > 1 {
		var uptime time.Duration
		var downtime time.Duration
		recentTime := info.Entry.ScanHistory[0].Timestamp
		recentSuccess := info.Entry.ScanHistory[0].Success
		for _, scan := range info.Entry.ScanHistory[1:] {
			if recentSuccess {
				uptime += scan.Timestamp.Sub(recentTime)
			} else {
				downtime += scan.Timestamp.Sub(recentTime)
			}
			recentTime = scan.Timestamp
			recentSuccess = scan.Success
		}
		uptimeRatio = float64(uptime) / float64(uptime+downtime)
	}

	// Compute the uptime ratio, but shift by 0.02 to acknowledge fully that
	// 98% uptime and 100% uptime is valued the same.
	fmt.Println("\n  Scan History Length:", len(info.Entry.ScanHistory))
	fmt.Printf("  Overall Uptime:      %.3f\n", uptimeRatio)

	fmt.Println()
}
