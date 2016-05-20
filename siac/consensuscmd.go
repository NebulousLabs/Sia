package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	consensusCmd = &cobra.Command{
		Use:   "consensus",
		Short: "Print the current state of consensus",
		Long:  "Print the current state of consensus such as current block, block height, and target.",
		Run:   wrap(consensuscmd),
	}
)

// consensuscmd is the handler for the command `siac consensus`.
// Prints the current state of consensus.
func consensuscmd() {
	var cg api.ConsensusGET
	err := getAPI("/consensus", &cg)
	if err != nil {
		die("Could not get current consensus state:", err)
	}
	if cg.Synced {
		fmt.Printf(`Synced: %v
Block:  %v
Height: %v
Target: %v
`, yesNo(cg.Synced), cg.CurrentBlock, cg.Height, cg.Target)
	} else {
		// Estimate the height of the blockchain by calculating the minutes since a
		// known block, and dividing by 10 minutes (the block time).
		block50000Timestamp := time.Date(2016, time.May, 11, 19, 33, 0, 0, time.UTC)
		diff := time.Since(block50000Timestamp)
		estimatedHeight := 50000 + (diff.Minutes() / 10)
		estimatedProgress := float64(cg.Height) / estimatedHeight * 100
		fmt.Printf(`Synced: %v
Height: %v
Progress (estimated): %.f%%
`, yesNo(cg.Synced), cg.Height, estimatedProgress)
	}
}
