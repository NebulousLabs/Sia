package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	hostCmd = &cobra.Command{
		Use:   "host [config|setconfig]",
		Short: "Perform host actions",
		Long:  "View or modify host settings. Modifying host settings also announces the host to the network.",
		Run:   wrap(hostconfigcmd),
	}

	hostConfigCmd = &cobra.Command{
		Use:   "host config",
		Short: "View host settings",
		Long:  "View host settings, including available storage, price, and more.",
		Run:   wrap(hostconfigcmd),
	}

	hostSetConfigCmd = &cobra.Command{
		Use:   "host setconfig",
		Short: "Modify host settings",
		Long:  "Modify host settings, including available storage, price, and more. The new settings will be be announced to the network.",
		Run:   wrap(hostsetconfigcmd),
	}
)

func hostconfigcmd() {
	config, err := getHostConfig()
	if err != nil {
		fmt.Println("Could not fetch host settings:", err)
	}
}

/*
func hostsetconfigcmd(? string) {
	_, err := getResponse("/host", &url.Values{
	// "MB":           {args[0]},
	// "price":        {args[1]},
	// "freezecoins":  {args[2]},
	// "freezeblocks": {args[3]},
	})
	if err != nil {
		fmt.Println("Could not set host settings:", err)
		return
	}
	fmt.Println("Host settings set. You have been announced as a host on the network.")
}
*/
