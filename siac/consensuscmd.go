package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/types"
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
		estimatedHeight := estimatedHeightAt(time.Now())
		estimatedProgress := float64(cg.Height) / float64(estimatedHeight) * 100
		if estimatedProgress > 99 {
			estimatedProgress = 99
		}
		fmt.Printf(`Synced: %v
Height: %v
Progress (estimated): %.f%%
`, yesNo(cg.Synced), cg.Height, estimatedProgress)
	}
}

// estimatedHeightAt returns the estimated block height for the given time.
// Block height is estimated by calculating the minutes since a known block in
// the past and dividing by 10 minutes (the block time).
func estimatedHeightAt(t time.Time) types.BlockHeight {
	block5e4Timestamp := time.Date(2016, time.May, 11, 19, 33, 0, 0, time.UTC)
	diff := t.Sub(block5e4Timestamp)
	estimatedHeight := 5e4 + (diff.Minutes() / 10)
	return types.BlockHeight(estimatedHeight + 0.5) // round to the nearest block
}
