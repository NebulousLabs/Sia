package main

import (
	"fmt"

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

	// DEPRECATED v0.5.0
	consensusDeprecatedStatusCmd = &cobra.Command{
		Use:        "status",
		Deprecated: "use `siac` or `siac consensus` instead.",
		Run:        wrap(consensuscmd),
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
	fmt.Printf(`Block:  %v
Height: %v
Target: %v
`, cg.CurrentBlock, cg.Height, cg.Target)
}
