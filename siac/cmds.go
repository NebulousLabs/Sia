package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"

	"github.com/spf13/cobra"
)

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
	http.Get("http://localhost:9980/stop")
}

func minecmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	// TODO: need start/stop
	resp, err := http.Get("http://localhost:9980/mine")
	resp.Body.Close()
	if err != nil {
		fmt.Println(err)
	}
}

func sendcmd(cmd *cobra.Command, args []string) {
	if len(args) != 3 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm("http://localhost:9980/send", url.Values{
		"amount": {args[0]},
		"fee":    {args[1]},
		"dest":   {args[2]},
	})
}

func hostcmd(cmd *cobra.Command, args []string) {
	if len(args) != 4 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm("http://localhost:9980/host", url.Values{
		"MB":           {args[0]},
		"price":        {args[1]},
		"freezecoins":  {args[2]},
		"freezeblocks": {args[3]},
	})
}

func rentcmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm("http://localhost:9980/rent", url.Values{
		"filename": {args[0]},
	})
}

func downloadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm("http://localhost:9980/download", url.Values{
		"filename": {args[0]},
	})
}

func savecmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm("http://localhost:9980/save", url.Values{
		"filename": {args[0]},
	})
}

func loadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm("http://localhost:9980/load", url.Values{
		"filename":   {args[0]},
		"friendname": {args[1]},
	})
}

func statuscmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	resp, err := http.Get("http://localhost:9980/status")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	all, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(all))
	}
}
