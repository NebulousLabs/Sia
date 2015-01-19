package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/sia/components"
)

var (
	hostCmd = &cobra.Command{
		Use:   "host",
		Short: "Perform host actions",
		Long:  "View or modify host settings. Modifying host settings also announces the host to the network.",
		Run:   wrap(hostconfigcmd),
	}

	hostConfigCmd = &cobra.Command{
		Use:   "config",
		Short: "View host settings",
		Long:  "View host settings, including available storage, price, and more.",
		Run:   wrap(hostconfigcmd),
	}

	hostSetConfigCmd = &cobra.Command{
		Use:   "setconfig [totalstorage] [maxfilesize] [mintolerance] [maxduration] [price] [burn]",
		Short: "Modify host settings",
		Long:  "Modify host settings, including available storage, price, and more. The new settings will be be announced to the network.",
		Run:   wrap(hostsetconfigcmd),
	}
)

func hostconfigcmd() {
	config := new(components.HostInfo)
	err := getAPI("/host/config", config)
	if err != nil {
		fmt.Println("Could not fetch host settings:", err)
		return
	}
	fmt.Printf(`Host settings:
Storage:      %v bytes (%v remaining)
Price:        %v coins
Max Filesize: %v
Max Duration: %v
Tolerance:    %v
Burn:         %v
`, config.Announcement.TotalStorage, config.StorageRemaining, config.Announcement.Price, config.Announcement.MaxFilesize,
		config.Announcement.MaxDuration, config.Announcement.MinTolerance, config.Announcement.Burn)
}

// TODO: settings should be updated individually, then submitted together in a
// separate API call.
func hostsetconfigcmd(totalstorage, maxfilesize, mintolerance, maxduration, price, burn string) {
	err := callAPI(fmt.Sprintf("/host/setconfig?totalstorage=%s&maxfilesize=%s&mintolerance=%s"+
		"&maxduration=%s&price=%s&burn=%s", totalstorage, maxfilesize, mintolerance, maxduration, price, burn))
	if err != nil {
		fmt.Println("Could not update host settings:", err)
		return
	}
	fmt.Println("Host settings updated. You have been announced as a host on the network.")
}
