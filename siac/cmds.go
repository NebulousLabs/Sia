package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

var hostname = "http://localhost:9980"

// helper function for reading http GET responses
func getResponse(handler string, vals *url.Values) (map[string]interface{}, error) {
	// create query string, if supplied
	qs := ""
	if vals != nil {
		qs = "?" + vals.Encode()
	}
	// send GET request
	// TODO: timeout?
	resp, err := http.Get(hostname + handler + qs)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer resp.Body.Close()

	// decode response
	obj := make(map[string]interface{})
	// don't capture this error; we only care about HTTP errors
	json.NewDecoder(resp.Body).Decode(&obj)
	return obj, nil
}

func stopcmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		cmd.Usage()
		return
	}
	getResponse("/stop", nil)
	fmt.Println("Sia daemon stopped")
}

func updatecmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		cmd.Usage()
		return
	}
	switch args[0] {
	case "check":
		if m, err := getResponse("/update/check", nil); err == nil {
			if m["Available"].(bool) {
				fmt.Printf("Update %s is available!\nRun 'siac update %s' to install it.\n", m["Version"], m["Version"])
			} else {
				fmt.Println("Up to date!")
			}
		} else {
			fmt.Println("Update check failed:", err)
		}

	default:
		_, err := getResponse("/update/apply", &url.Values{
			"version": {args[0]},
		})
		if err == nil {
			fmt.Printf("Update %s applied!\nYou must restart siad for changes to take effect.\n", args[0])
		} else {
			fmt.Println("Couldn't apply update:", err)
		}
	}
}

func minecmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cmd.Usage()
		return
	}
	switch args[0] {
	case "start":
		_, err := getResponse("/miner/start", &url.Values{
			"threads": {args[1]},
		})
		if err != nil {
			fmt.Println("Could not start miner:", err)
		} else {
			fmt.Println("Mining on " + args[1] + " threads.")
		}
	case "stop":
		_, err := getResponse("/miner/stop", nil)
		if err != nil {
			fmt.Println("Could not stop miner:", err)
		} else {
			fmt.Println("Stopped mining.")
		}

	case "status":
		m, err := getResponse("/miner/status", nil)
		if err != nil {
			fmt.Println("Could not get miner status:", err)
		} else {
			fmt.Println(m)
		}

	default:
		cmd.Usage()
	}
}

func synccmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		cmd.Usage()
		return
	}
	if _, err := getResponse("/sync", nil); err == nil {
		fmt.Println("Sync initiated.")
	} else {
		fmt.Println(err)
	}
}

func peercmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cmd.Usage()
		return
	}
	_, err := getResponse("/peer/"+args[0], &url.Values{
		"addr": {args[1]},
	})
	if err != nil {
		fmt.Println("Couldn't modify peer:", err)
	}
}

func sendcmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cmd.Usage()
		return
	}
	fmt.Println(getResponse("/wallet/send", &url.Values{
		"amount": {args[0]},
		"dest":   {args[1]},
	}))
}

func hostcmd(cmd *cobra.Command, args []string) {
	if len(args) != 4 {
		cmd.Usage()
		return
	}
	_, err := getResponse("/host", &url.Values{
	// "MB":           {args[0]},
	// "price":        {args[1]},
	// "freezecoins":  {args[2]},
	// "freezeblocks": {args[3]},
	})
	if err != nil {
		fmt.Println("Could not submit host settings:", err)
	} else {
		fmt.Println("Host settings submitted.")
	}
}

func rentcmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cmd.Usage()
		return
	}
	_, err := getResponse("/rent", &url.Values{
		"filename": {args[0]},
		"nickname": {args[1]},
	})
	if err == nil {
		fmt.Println("Uploaded " + args[0] + ".")
	} else {
		fmt.Println("Upload failed:", err)
	}
}

func downloadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cmd.Usage()
		return
	}
	_, err := getResponse("/download", &url.Values{
		"nickname":    {args[0]},
		"destination": {args[1]},
	})
	if err == nil {
		fmt.Println("Downloaded " + args[1] + ".")
	} else {
		fmt.Println("Download failed:", err)
	}
}

func statuscmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		cmd.Usage()
		return
	}
	m, err := getResponse("/status", nil)
	if err != nil {
		fmt.Println("Could not get status:", err)
	} else {
		fmt.Println(m)
	}
}
