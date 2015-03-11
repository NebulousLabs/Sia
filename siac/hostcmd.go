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
		Use:   "set [setting] [value]",
		Short: "Modify host settings",
		Long: `Modify host settings.
Available settings:
	totalstorage
	maxfilesize
	mintolerance
	maxduration
	price
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
	err := callAPI(fmt.Sprintf("/host/config?%s=%s", param, value))
	if err != nil {
		fmt.Println("Could not update host settings:", err)
		return
	}
	fmt.Println("Host settings updated.")
}

func hostannouncecmd() {
	err := callAPI("/host/announce")
	if err != nil {
		fmt.Println("Could not announce host:", err)
		return
	}
	fmt.Println("Host announcement submitted to network.")
}

func hoststatuscmd() {
	config := new(modules.HostInfo)
	err := getAPI("/host/config", config)
	if err != nil {
		fmt.Println("Could not fetch host settings:", err)
		return
	}
	fmt.Printf(`Host settings:
Storage:      %v bytes (%v remaining)
Price:        %v coins
Collateral:   %v
Max Filesize: %v
Max Duration: %v
`, config.TotalStorage, config.StorageRemaining, config.Price, config.Collateral, config.MaxFilesize, config.MaxDuration)
}
