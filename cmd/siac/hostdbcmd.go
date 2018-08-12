package main

import (
	"fmt"
	"math/big"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

const scanHistoryLen = 30

var (
	hostdbNumHosts int
	hostdbVerbose  bool
)

var (
	hostdbCmd = &cobra.Command{
		Use:   "hostdb",
		Short: "Interact with the renter's host database.",
		Long:  "View the list of active hosts, the list of all hosts, or query specific hosts.\nIf the '-v' flag is set, a list of recent scans will be provided, with the most\nrecent scan on the right. a '0' indicates that the host was offline, and a '1'\nindicates that the host was online.",
		Run:   wrap(hostdbcmd),
	}

	hostdbViewCmd = &cobra.Command{
		Use:   "view [pubkey]",
		Short: "View the full information for a host.",
		Long:  "View detailed information about a host, including things like a score breakdown.",
		Run:   wrap(hostdbviewcmd),
	}
)

// printScoreBreakdown prints the score breakdown of a host, provided the info.
func printScoreBreakdown(info *api.HostdbHostsGET) {
	fmt.Println("\n  Score Breakdown:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "\t\tAge:\t %.3f\n", info.ScoreBreakdown.AgeAdjustment)
	fmt.Fprintf(w, "\t\tBurn:\t %.3f\n", info.ScoreBreakdown.BurnAdjustment)
	fmt.Fprintf(w, "\t\tCollateral:\t %.3f\n", info.ScoreBreakdown.CollateralAdjustment)
	fmt.Fprintf(w, "\t\tInteraction:\t %.3f\n", info.ScoreBreakdown.InteractionAdjustment)
	fmt.Fprintf(w, "\t\tPrice:\t %.3f\n", info.ScoreBreakdown.PriceAdjustment*1e6)
	fmt.Fprintf(w, "\t\tStorage:\t %.3f\n", info.ScoreBreakdown.StorageRemainingAdjustment)
	fmt.Fprintf(w, "\t\tUptime:\t %.3f\n", info.ScoreBreakdown.UptimeAdjustment)
	fmt.Fprintf(w, "\t\tVersion:\t %.3f\n", info.ScoreBreakdown.VersionAdjustment)
	w.Flush()
}

func hostdbcmd() {
	if !hostdbVerbose {
		info, err := httpClient.HostDbActiveGet()
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
		fmt.Fprintln(w, "\t\tAddress\tPrice (per TB per Mo)")
		for i, host := range info.Hosts {
			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v\n", len(info.Hosts)-i, host.NetAddress, currencyUnits(price))
		}
		w.Flush()
	} else {
		info, err := httpClient.HostDbAllGet()
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

		fmt.Println()
		fmt.Println(len(offlineHosts), "Offline Hosts:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\t\tPubkey\tAddress\tPrice (/ TB / Month)\tDownload Price (/ TB)\tUptime\tRecent Scans")
		for i, host := range offlineHosts {
			// Compute the total measured uptime and total measured downtime for this
			// host.
			uptimeRatio := float64(0)
			if len(host.ScanHistory) > 1 {
				downtime := host.HistoricDowntime
				uptime := host.HistoricUptime
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

			// Get the scan history string.
			scanHistStr := ""
			displayScans := host.ScanHistory
			if len(host.ScanHistory) > scanHistoryLen {
				displayScans = host.ScanHistory[len(host.ScanHistory)-scanHistoryLen:]
			}
			for _, scan := range displayScans {
				if scan.Success {
					scanHistStr += "1"
				} else {
					scanHistStr += "0"
				}
			}

			// Get a string representation of the historic outcomes of the most
			// recent scans.
			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			downloadBWPrice := host.StoragePrice.Mul(modules.BytesPerTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v\t%v\t%v\t%.3f\t%s\n", len(offlineHosts)-i, host.PublicKeyString,
				host.NetAddress, currencyUnits(price), currencyUnits(downloadBWPrice), uptimeRatio, scanHistStr)
		}
		w.Flush()

		fmt.Println()
		fmt.Println(len(inactiveHosts), "Inactive Hosts:")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\t\tPubkey\tAddress\tPrice (/ TB / Month)\tDownload Price (/ TB)\tUptime\tRecent Scans")
		for i, host := range inactiveHosts {
			// Compute the total measured uptime and total measured downtime for this
			// host.
			uptimeRatio := float64(0)
			if len(host.ScanHistory) > 1 {
				downtime := host.HistoricDowntime
				uptime := host.HistoricUptime
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

			// Get a string representation of the historic outcomes of the most
			// recent scans.
			scanHistStr := ""
			displayScans := host.ScanHistory
			if len(host.ScanHistory) > scanHistoryLen {
				displayScans = host.ScanHistory[len(host.ScanHistory)-scanHistoryLen:]
			}
			for _, scan := range displayScans {
				if scan.Success {
					scanHistStr += "1"
				} else {
					scanHistStr += "0"
				}
			}

			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			downloadBWPrice := host.DownloadBandwidthPrice.Mul(modules.BytesPerTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v\t%v\t%v\t%.3f\t%s\n", len(inactiveHosts)-i, host.PublicKeyString, host.NetAddress, currencyUnits(price), currencyUnits(downloadBWPrice), uptimeRatio, scanHistStr)
		}
		w.Flush()

		// Grab the host at the 1/5th point and use it as the reference. (it's
		// like using the median, except at the 1/5th point instead of the 1/2
		// point.)
		referenceScore := big.NewRat(1, 1)
		if len(activeHosts) > 0 {
			referenceIndex := len(activeHosts) / 5
			hostInfo, err := httpClient.HostDbHostsGet(activeHosts[referenceIndex].PublicKey)
			if err != nil {
				die("Could not fetch provided host:", err)
			}
			if !hostInfo.ScoreBreakdown.Score.IsZero() {
				referenceScore = new(big.Rat).Inv(new(big.Rat).SetInt(hostInfo.ScoreBreakdown.Score.Big()))
			}
		}

		fmt.Println()
		fmt.Println(len(activeHosts), "Active Hosts:")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "\t\tPubkey\tAddress\tScore\tPrice (/ TB / Month)\tDownload Price (/TB)\tUptime\tRecent Scans")
		for i, host := range activeHosts {
			// Compute the total measured uptime and total measured downtime for this
			// host.
			uptimeRatio := float64(0)
			if len(host.ScanHistory) > 1 {
				downtime := host.HistoricDowntime
				uptime := host.HistoricUptime
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

			// Get a string representation of the historic outcomes of the most
			// recent scans.
			scanHistStr := ""
			displayScans := host.ScanHistory
			if len(host.ScanHistory) > scanHistoryLen {
				displayScans = host.ScanHistory[len(host.ScanHistory)-scanHistoryLen:]
			}
			for _, scan := range displayScans {
				if scan.Success {
					scanHistStr += "1"
				} else {
					scanHistStr += "0"
				}
			}

			// Grab the score information for the active hosts.
			hostInfo, err := httpClient.HostDbHostsGet(host.PublicKey)
			if err != nil {
				die("Could not fetch provided host:", err)
			}
			score, _ := new(big.Rat).Mul(referenceScore, new(big.Rat).SetInt(hostInfo.ScoreBreakdown.Score.Big())).Float64()

			price := host.StoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)
			downloadBWPrice := host.DownloadBandwidthPrice.Mul(modules.BytesPerTerabyte)
			fmt.Fprintf(w, "\t%v:\t%v\t%v\t%12.6g\t%v\t%v\t%.3f\t%s\n", len(activeHosts)-i, host.PublicKeyString, host.NetAddress, score, currencyUnits(price), currencyUnits(downloadBWPrice), uptimeRatio, scanHistStr)
		}
		w.Flush()
	}
}

func hostdbviewcmd(pubkey string) {
	var publicKey types.SiaPublicKey
	publicKey.LoadString(pubkey)
	info, err := httpClient.HostDbHostsGet(publicKey)
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

	printScoreBreakdown(&info)

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
