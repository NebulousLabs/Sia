package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"

	"github.com/spf13/cobra"
)

var hostname = "http://localhost:9980"

// helper function for reading http GET responses
func getResponse(handler string, vals *url.Values) string {
	// create query string, if supplied
	qs := "?"
	if vals != nil {
		qs += vals.Encode()
	}
	// send GET request
	resp, err := http.Get(hostname + handler + qs)
	if err != nil {
		return err.Error()
	}
	// read response
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func startcmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	// TODO: specify port
	// TODO: don't start if already started
	exec.Command("siad")
}

func stopcmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	getResponse("/stop", nil)
}

func minecmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	// TODO: need start/stop
	getResponse("/mine", nil)
}

func sendcmd(cmd *cobra.Command, args []string) {
	if len(args) != 3 {
		fmt.Println(cmd.Usage())
		return
	}
	fmt.Println(getResponse("/send", &url.Values{
		"amount": {args[0]},
		"fee":    {args[1]},
		"dest":   {args[2]},
	}))
}

func hostcmd(cmd *cobra.Command, args []string) {
	if len(args) != 4 {
		fmt.Println(cmd.Usage())
		return
	}
	fmt.Println(getResponse("/host", &url.Values{
		"MB":           {args[0]},
		"price":        {args[1]},
		"freezecoins":  {args[2]},
		"freezeblocks": {args[3]},
	}))
}

func rentcmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	fmt.Println(getResponse("/rent", &url.Values{
		"filename": {args[0]},
	}))
}

func downloadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	fmt.Println(getResponse("/download", &url.Values{
		"filename": {args[0]},
	}))
}

func savecmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	fmt.Println(getResponse("/save", &url.Values{
		"filename": {args[0]},
	}))
}

func loadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fmt.Println(cmd.Usage())
		return
	}
	fmt.Println(getResponse("/load", &url.Values{
		"filename":   {args[0]},
		"friendname": {args[1]},
	}))
}

func statuscmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	fmt.Println(getResponse("/status", nil))
}
