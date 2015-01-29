package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules/host"
)

var (
	hostCmd = &cobra.Command{
		Use:   "host",
		Short: "Perform host actions",
		Long:  "View or modify host settings. Modifying host settings also announces the host to the network.",
		Run:   wrap(hoststatuscmd),
	}

	hostSetCmd = &cobra.Command{
		Use:   "set [setting] [value]",
		Short: "Modify host settings",
		Long:  "Modify host settings.\nAvailable settings:\n\ttotalstorage\n\tmaxfilesize\n\tmintolerance\n\tmaxduration\n\tprice\n\tburn",
		Run:   wrap(hostsetcmd),
	}

	hostAnnounceCmd = &cobra.Command{
		Use:   "announce",
		Short: "Announce host",
		Long:  "Announce yourself as a host on the network. You may wish to set your hosting parameters first, via 'host setconfig'.",
		Run:   wrap(hostannouncecmd),
	}

	hostStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View host settings",
		Long:  "View host settings, including available storage, price, and more.",
		Run:   wrap(hoststatuscmd),
	}
)

func hostsetcmd(param, value string) {
	err := callAPI(fmt.Sprintf("/host/setconfig?%s=%s", param, value))
	if err != nil {
		fmt.Println("Could not update host settings:", err)
		return
	}
	fmt.Println("Host settings updated.")
}

// TODO: needs freeze values
func hostannouncecmd() {
	err := callAPI("/host/announce")
	if err != nil {
		fmt.Println("Could not announce host:", err)
		return
	}
	fmt.Println("Host announcement submitted to network.")
}

func hoststatuscmd() {
	config := new(host.HostInfo)
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
Burn:         %v
`, config.TotalStorage, config.StorageRemaining, config.Price, config.MaxFilesize, config.MaxDuration, config.Burn)
}
