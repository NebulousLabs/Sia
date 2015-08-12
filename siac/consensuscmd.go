package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	consensusStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Print the current state of the daemon",
		Long:  "Query the daemon for values such as the current difficulty, target, height, peers, transactions, etc.",
		Run:   wrap(consensusstatuscmd),
	}
)

func consensusstatuscmd() {
	var cg api.ConsensusGET
	err := getAPI("/consensus", &cg)
	if err != nil {
		fmt.Println("Could not get daemon status:", err)
		return
	}
	fmt.Printf(`Block:  %v
Height: %v
Target: %v
`, cg.CurrentBlock, cg.Height, cg.Target)
}
