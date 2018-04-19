package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

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
	cg, err := httpClient.ConsensusGet()
	if err != nil {
		die("Could not get current consensus state:", err)
	}
	if cg.Synced {
		fmt.Printf(`Synced: %v
Block:      %v
Height:     %v
Target:     %v
Difficulty: %v
`, yesNo(cg.Synced), cg.CurrentBlock, cg.Height, cg.Target, cg.Difficulty)
	} else {
		estimatedHeight := estimatedHeightAt(time.Now())
		estimatedProgress := float64(cg.Height) / float64(estimatedHeight) * 100
		if estimatedProgress > 100 {
			estimatedProgress = 100
		}
		fmt.Printf(`Synced: %v
Height: %v
Progress (estimated): %.1f%%
`, yesNo(cg.Synced), cg.Height, estimatedProgress)
	}
}

// estimatedHeightAt returns the estimated block height for the given time.
// Block height is estimated by calculating the minutes since a known block in
// the past and dividing by 10 minutes (the block time).
func estimatedHeightAt(t time.Time) types.BlockHeight {
	block100kTimestamp := time.Date(2017, time.April, 13, 23, 29, 49, 0, time.UTC)
	blockTime := float64(9) // overestimate block time for better UX
	diff := t.Sub(block100kTimestamp)
	estimatedHeight := 100e3 + (diff.Minutes() / blockTime)
	return types.BlockHeight(estimatedHeight + 0.5) // round to the nearest block
}
