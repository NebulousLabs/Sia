package main

import (
	"fmt"
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
	http.Get(":9980/stop")
}

func minecmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	// TODO: need start/stop
	http.Get(":9980/mine")
}

func sendcmd(cmd *cobra.Command, args []string) {
	if len(args) != 3 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm(":9980/send", url.Values{
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
	http.PostForm(":9980/host", url.Values{
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
	http.PostForm(":9980/rent", url.Values{
		"filename": {args[0]},
	})
}

func downloadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm(":9980/rent", url.Values{
		"filename": {args[0]},
	})
}

func savecmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm(":9980/rent", url.Values{
		"filename": {args[0]},
	})
}

func loadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fmt.Println(cmd.Usage())
		return
	}
	http.PostForm(":9980/rent", url.Values{
		"filename":   {args[0]},
		"friendname": {args[1]},
	})
}

func statscmd(cmd *cobra.Command, args []string) {
	if len(args) != 0 {
		fmt.Println(cmd.Usage())
		return
	}
	http.Get(":9980/stats")
}
