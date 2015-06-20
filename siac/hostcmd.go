package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules"
)

var (
	hostCmd = &cobra.Command{
		Use:   "host",
		Short: "Perform host actions",
		Long:  "View or modify host settings. Modifying host settings also announces the host to the network.",
		Run:   wrap(hoststatuscmd),
	}

	hostConfigCmd = &cobra.Command{
		Use:   "config [setting] [value]",
		Short: "Modify host settings",
		Long: `Modify host settings.
Available settings:
	totalstorage
	minfilesize
	maxfilesize
	minduration
	maxduration
	windowsize
	price (in hastings)
	collateral`,
		Run: wrap(hostconfigcmd),
	}

	hostAnnounceCmd = &cobra.Command{
		Use:   "announce",
		Short: "Announce yourself as a host",
		Long:  "Announce yourself as a host on the network.",
		Run:   wrap(hostannouncecmd)}

	hostStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View host settings",
		Long:  "View host settings, including available storage, price, and more.",
		Run:   wrap(hoststatuscmd),
	}
)

func hostconfigcmd(param, value string) {
	err := post("/host/configure", param+"="+value)
	if err != nil {
		fmt.Println("Could not update host settings:", err)
		return
	}
	fmt.Println("Host settings updated.")
}

func hostannouncecmd() {
	err := post("/host/announce", "")
	if err != nil {
		fmt.Println("Could not announce host:", err)
		return
	}
	fmt.Println("Host announcement submitted to network.")
}

func hoststatuscmd() {
	info := new(modules.HostInfo)
	err := getAPI("/host/status", info)
	if err != nil {
		fmt.Println("Could not fetch host settings:", err)
		return
	}
	fmt.Printf(`Host settings:
Storage:      %v (%v used)
Price:        %v coins
Collateral:   %v
Max Filesize: %v
Max Duration: %v
Contracts:    %v
`, filesizeUnits(info.TotalStorage), filesizeUnits(info.TotalStorage-info.StorageRemaining), info.Price, info.Collateral, info.MaxFilesize, info.MaxDuration, info.NumContracts)
}
