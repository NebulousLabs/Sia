package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	hostdbNumHosts int
	hostdbVerbose  bool
)

var (
	hostdbCmd = &cobra.Command{
		Use:   "hostdb",
		Short: "Interact with the renter's host database.",
		Long:  "View the list of active hosts, the list of all hosts, or query specific hosts.",
		Run:   wrap(hostdbcmd),
	}

	hostdbViewCmd = &cobra.Command{
		Use:   "view [pubkey]",
		Short: "View the full information for a host.",
		Long:  "View detailed information about a host, including things like a score breakdown.",
		Run:   wrap(hostdbviewcmd),
	}
)

func hostdbcmd() {
	if !hostdbVerbose {
		info := new(api.HostdbActiveGET)
		err := getAPI("/hostdb/active", info)
		if err != nil {
			die("Could not fetch host list:", err)
		}
		if len(info.Hosts) == 0 {
			fmt.Println("No known active hosts")
			return
		}

		// Strip down to the number of requested hosts.
		if hostdbNumHosts != 0 && hostdbNumHosts < len(info.Hosts) {
			info.Hosts = info.Hosts[len(info.Hosts)-hostdbNumHosts:]
		}

		fmt.Println(len(info.Hosts), "Active Hosts:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\t\tAddress\tPrice\t")
		for i, host := range info.Hosts {
			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v \t (per TB per Month)\n", len(info.Hosts)-i, host.NetAddress, currencyUnits(price))
		}
		w.Flush()
	} else {
		info := new(api.HostdbAllGET)
		err := getAPI("/hostdb/all", info)
		if err != nil {
			die("Could not fetch host list:", err)
		}
		if len(info.Hosts) == 0 {
			fmt.Println("No known hosts")
			return
		}

		// Iterate through the hosts and divide by category.
		var activeHosts, inactiveHosts, offlineHosts []api.ExtendedHostDBEntry
		for _, host := range info.Hosts {
			if host.AcceptingContracts && len(host.ScanHistory) > 0 && host.ScanHistory[len(host.ScanHistory)-1].Success {
				activeHosts = append(activeHosts, host)
				continue
			}
			if len(host.ScanHistory) > 0 && host.ScanHistory[len(host.ScanHistory)-1].Success {
				inactiveHosts = append(inactiveHosts, host)
				continue
			}
			offlineHosts = append(offlineHosts, host)
		}

		if hostdbNumHosts > 0 && len(offlineHosts) > hostdbNumHosts {
			offlineHosts = offlineHosts[len(offlineHosts)-hostdbNumHosts:]
		}
		if hostdbNumHosts > 0 && len(inactiveHosts) > hostdbNumHosts {
			inactiveHosts = inactiveHosts[len(inactiveHosts)-hostdbNumHosts:]
		}
		if hostdbNumHosts > 0 && len(activeHosts) > hostdbNumHosts {
			activeHosts = activeHosts[len(activeHosts)-hostdbNumHosts:]
		}

		fmt.Println(len(offlineHosts), "Offline Hosts:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\t\tPubkey\tAddress\tPrice\t\tUptime")
		for i, host := range offlineHosts {
			// Compute the total measured uptime and total measured downtime for this
			// host.
			uptimeRatio := float64(0)
			if len(host.ScanHistory) > 1 {
				var uptime time.Duration
				var downtime time.Duration
				recentTime := host.ScanHistory[0].Timestamp
				recentSuccess := host.ScanHistory[0].Success
				for _, scan := range host.ScanHistory[1:] {
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

			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v \t(per TB per Month)\t%v\t%.3f\n", len(offlineHosts)-i, host.PublicKeyString, host.NetAddress, currencyUnits(price), uptimeRatio)
		}
		w.Flush()

		fmt.Println()
		fmt.Println(len(inactiveHosts), "Inactive Hosts:")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\t\tPubkey\tAddress\tPrice\t\tUptime")
		for i, host := range inactiveHosts {
			// Compute the total measured uptime and total measured downtime for this
			// host.
			uptimeRatio := float64(0)
			if len(host.ScanHistory) > 1 {
				var uptime time.Duration
				var downtime time.Duration
				recentTime := host.ScanHistory[0].Timestamp
				recentSuccess := host.ScanHistory[0].Success
				for _, scan := range host.ScanHistory[1:] {
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

			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v \t(per TB per Month)\t%v\t%.3f\n", len(inactiveHosts)-i, host.PublicKeyString, host.NetAddress, currencyUnits(price), uptimeRatio)
		}
		w.Flush()

		fmt.Println()
		fmt.Println(len(activeHosts), "Active Hosts:")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\t\tPubkey\tAddress\tPrice\t\tUptime")
		for i, host := range activeHosts {
			// Compute the total measured uptime and total measured downtime for this
			// host.
			uptimeRatio := float64(0)
			if len(host.ScanHistory) > 1 {
				var uptime time.Duration
				var downtime time.Duration
				recentTime := host.ScanHistory[0].Timestamp
				recentSuccess := host.ScanHistory[0].Success
				for _, scan := range host.ScanHistory[1:] {
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

			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v \t(per TB per Month)\t%v\t%.3f\n", len(activeHosts)-i, host.PublicKeyString, host.NetAddress, currencyUnits(price), uptimeRatio)
		}
		w.Flush()
	}
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
	fmt.Fprintln(w, "\t\tOffered Collateral (TB / Mo):\t", currencyUnits(info.Entry.Collateral.Mul(modules.BlockBytesPerMonthTerabyte)))
	fmt.Fprintln(w, "\n\t\tContract Price:\t", currencyUnits(info.Entry.ContractPrice))
	fmt.Fprintln(w, "\t\tStorage Price (TB / Mo):\t", currencyUnits(info.Entry.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)))
	fmt.Fprintln(w, "\t\tDownload Price (1 TB):\t", currencyUnits(info.Entry.DownloadBandwidthPrice.Mul(modules.BytesPerTerabyte)))
	fmt.Fprintln(w, "\t\tUpload Price (1 TB):\t", currencyUnits(info.Entry.UploadBandwidthPrice.Mul(modules.BytesPerTerabyte)))
	fmt.Fprintln(w, "\t\tVersion:\t", info.Entry.Version)
	w.Flush()

	fmt.Println("\n  Score Breakdown:")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	total := info.ScoreBreakdown.AgeAdjustment * info.ScoreBreakdown.BurnAdjustment * info.ScoreBreakdown.CollateralAdjustment * info.ScoreBreakdown.PriceAdjustment * info.ScoreBreakdown.StorageRemainingAdjustment * info.ScoreBreakdown.UptimeAdjustment * info.ScoreBreakdown.VersionAdjustment * 1e12
	fmt.Fprintf(w, "\t\tTotal Score:\t %.0f\n\n", total)
	fmt.Fprintf(w, "\t\tAge:\t %.3f\n", info.ScoreBreakdown.AgeAdjustment)
	fmt.Fprintf(w, "\t\tBurn:\t %.3f\n", info.ScoreBreakdown.BurnAdjustment)
	fmt.Fprintf(w, "\t\tCollateral:\t %.3f\n", info.ScoreBreakdown.CollateralAdjustment)
	fmt.Fprintf(w, "\t\tPrice:\t %.3f\n", info.ScoreBreakdown.PriceAdjustment*1e6)
	fmt.Fprintf(w, "\t\tStorage:\t %.3f\n", info.ScoreBreakdown.StorageRemainingAdjustment)
	fmt.Fprintf(w, "\t\tUptime:\t %.3f\n", info.ScoreBreakdown.UptimeAdjustment)
	fmt.Fprintf(w, "\t\tVersion:\t %.3f\n", info.ScoreBreakdown.VersionAdjustment)
	w.Flush()

	// Compute the total measured uptime and total measured downtime for this
	// host.
	uptimeRatio := float64(0)
	if len(info.Entry.ScanHistory) > 1 {
		downtime := info.Entry.HistoricDowntime
		uptime := info.Entry.HistoricUptime
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
