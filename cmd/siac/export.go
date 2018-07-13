package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/NebulousLabs/Sia/types"

	"github.com/spf13/cobra"
)

var (
	renterExportCmd = &cobra.Command{
		Use:   "export",
		Short: "export renter data to various formats",
		Long:  "Export renter data in various formats.",
		// Run field not provided; export requires a subcommand.
	}

	renterExportContractTxnsCmd = &cobra.Command{
		Use:   "contract-txns [destination]",
		Short: "export the renter's contracts for import to `https://rankings.sia.tech/`",
		Long: "Export the renter's current contract set in JSON format to the specified " +
			"file. Intended for upload to `https://rankings.sia.tech/`.",
		Run: wrap(renterexportcontracttxnscmd),
	}
)

// renterexportcontracttxnscmd is the handler for the command `siac renter export contract-txns`.
// Exports the current contract set to JSON.
func renterexportcontracttxnscmd(destination string) {
	cs, err := httpClient.RenterContractsGet()
	if err != nil {
		die("Could not retrieve contracts:", err)
	}
	var contractTxns []types.Transaction
	for _, c := range cs.Contracts {
		contractTxns = append(contractTxns, c.LastTransaction)
	}
	destination = abs(destination)
	file, err := os.Create(destination)
	if err != nil {
		die("Could not export to file:", err)
	}
	err = json.NewEncoder(file).Encode(contractTxns)
	if err != nil {
		die("Could not export to file:", err)
	}
	fmt.Println("Exported contract data to", destination)
}
