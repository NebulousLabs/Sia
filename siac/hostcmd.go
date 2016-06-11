package main

import (
	"fmt"
	"math/big"
	"os"
	"text/tabwriter"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/spf13/cobra"
)

var (
	hostCmd = &cobra.Command{
		Use:   "host",
		Short: "Perform host actions",
		Long:  "View or modify host settings.",
		Run:   wrap(hostcmd),
	}

	hostConfigCmd = &cobra.Command{
		Use:   "config [setting] [value]",
		Short: "Modify host settings",
		Long: `Modify host settings.
Available settings:

Parameter                        Unit

acceptingcontracts               boolean
collateral                       currency/TB
collateralbudget                 currency
maxcollateral                    currency
maxdownloadbatchsize             int
maxduration                      int
maxrevisebatchsize               int
mincontractprice                 currency
mindownloadbandwidthprice        currency/TB
minstorageprice                  currency/TB/month
minuploadbandwidthprice          currency/TB
netaddress                       string
windowsize                       int

Currency units can be specified, e.g. 10SC; run 'siac help wallet' for details.

For a description of each parameter, see doc/API.md.

To configure the host to accept new contracts, set acceptingcontracts to true:
	siac host config acceptingcontracts true
`,
		Run: wrap(hostconfigcmd),
	}

	hostAnnounceCmd = &cobra.Command{
		Use:   "announce",
		Short: "Announce yourself as a host",
		Long: `Announce yourself as a host on the network.
Announcing will also configure the host to start accepting contracts.
You can revert this by running:
	siac host config acceptingcontracts false
You may also supply a specific address to be announced, e.g.:
	siac host announce my-host-domain.com:9001
Doing so will override the standard connectivity checks.`,
		Run: hostannouncecmd,
	}

	hostFolderCmd = &cobra.Command{
		Use:   "folder",
		Short: "Add, remove, or resize a storage folder",
		Long:  "Add, remove, or resize a storage folder.",
	}

	hostFolderAddCmd = &cobra.Command{
		Use:   "add [path] [size]",
		Short: "Add a storage folder to the host",
		Long:  "Add a storage folder to the host, specifying how much data it should store",
		Run:   wrap(hostfolderaddcmd),
	}

	hostFolderRemoveCmd = &cobra.Command{
		Use:   "remove [path]",
		Short: "Remove a storage folder from the host",
		Long: `Remove a storage folder from the host. Note that this does not delete any
data; it will instead be distributed across the remaining storage folders.`,

		Run: wrap(hostfolderremovecmd),
	}

	hostFolderResizeCmd = &cobra.Command{
		Use:   "resize [path] [size]",
		Short: "Resize a storage folder",
		Long: `Change how much data a storage folder should store. If the new size is less
than what the folder is currently storing, data will be distributed across the
other storage folders.`,
		Run: wrap(hostfolderresizecmd),
	}

	hostSectorCmd = &cobra.Command{
		Use:   "sector",
		Short: "Add or delete a sector (add not supported)",
		Long: `Add or delete a sector. Adding is not currently supported. Note that
deleting a sector may impact host revenue.`,
	}

	hostSectorDeleteCmd = &cobra.Command{
		Use:   "delete [root]",
		Short: "Delete a sector",
		Long: `Delete a sector, identified by its Merkle root. Note that deleting a
sector may impact host revenue.`,
		Run: wrap(hostsectordeletecmd),
	}
)

// hostcmd is the handler for the command `siac host`.
// Prints info about the host and its storage folders.
func hostcmd() {
	hg := new(api.HostGET)
	err := getAPI("/host", &hg)
	if err != nil {
		die("Could not fetch host settings:", err)
	}
	sg := new(api.StorageGET)
	err = getAPI("/storage", &sg)
	if err != nil {
		die("Could not fetch storage info:", err)
	}

	es := hg.ExternalSettings
	fm := hg.FinancialMetrics
	is := hg.InternalSettings
	nm := hg.NetworkMetrics

	// calculate total storage available and remaining
	var totalstorage, storageremaining uint64
	for _, folder := range sg.StorageFolderMetadata {
		totalstorage += folder.Capacity
		storageremaining += folder.CapacityRemaining
	}

	// convert price from bytes/block to TB/Month
	price := currencyUnits(is.MinStoragePrice.Mul(modules.BlockBytesPerMonthTerabyte))
	// calculate total revenue
	totalRevenue := fm.ContractCompensation.
		Add(fm.StorageRevenue).
		Add(fm.DownloadBandwidthRevenue).
		Add(fm.UploadBandwidthRevenue)
	totalPotentialRevenue := fm.PotentialContractCompensation.
		Add(fm.PotentialStorageRevenue).
		Add(fm.PotentialDownloadBandwidthRevenue).
		Add(fm.PotentialUploadBandwidthRevenue)
	// determine the display method for the net address.
	netaddr := es.NetAddress
	if is.NetAddress == "" {
		netaddr += " (automatically determined)"
	} else {
		netaddr += " (manually specified)"
	}

	if hostVerbose {
		// describe net address
		fmt.Printf(`Host Internal Settings:
	acceptingcontracts:   %v
	maxduration:          %v Weeks
	maxdownloadbatchsize: %v
	maxrevisebatchsize:   %v
	netaddress:           %v
	windowsize:           %v Weeks

	collateral:       %v / TB / Month
	collateralbudget: %v 
	maxcollateral:    %v Per Contract

	mincontractprice:         %v
	mindownloadbandwithprice: %v / TB
	minstorageprice:          %v / TB / Month
	minuploadbandwidthprice:  %v / TB

Host Financials:
	Transaction Fee Compensation: %v
	Transaction Fee Expenses:     %v

	Storage Revenue:           %v
	Potential Storage Revenue: %v

	Locked Collateral: %v
	Risked Collateral: %v
	Lost Collateral:   %v

	Download Revenue:           %v
	Potential Download Revenue: %v
	Upload Revenue :            %v
	Potential Upload Revenue:   %v

RPC Stats:
	Error Calls:        %v
	Unrecognized Calls: %v
	Download Calls:     %v
	Renew Calls:        %v
	Revise Calls:       %v
	Settings Calls:     %v
	FormContract Calls: %v
`,
			yesNo(is.AcceptingContracts), periodUnits(is.MaxDuration),
			filesizeUnits(int64(is.MaxDownloadBatchSize)),
			filesizeUnits(int64(is.MaxReviseBatchSize)), netaddr,
			periodUnits(is.WindowSize),

			currencyUnits(is.Collateral.Mul(modules.BlockBytesPerMonthTerabyte)),
			currencyUnits(is.CollateralBudget),
			currencyUnits(is.MaxCollateral),

			currencyUnits(is.MinContractPrice),
			currencyUnits(is.MinDownloadBandwidthPrice.Mul(modules.BytesPerTerabyte)),
			currencyUnits(is.MinStoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)),
			currencyUnits(is.MinUploadBandwidthPrice.Mul(modules.BytesPerTerabyte)),

			currencyUnits(fm.ContractCompensation),
			currencyUnits(fm.TransactionFeeExpenses),

			currencyUnits(fm.StorageRevenue),
			currencyUnits(fm.PotentialStorageRevenue),

			currencyUnits(fm.LockedStorageCollateral),
			currencyUnits(fm.RiskedStorageCollateral),
			currencyUnits(fm.LostStorageCollateral),

			currencyUnits(fm.DownloadBandwidthRevenue),
			currencyUnits(fm.PotentialDownloadBandwidthRevenue),
			currencyUnits(fm.UploadBandwidthRevenue),
			currencyUnits(fm.PotentialUploadBandwidthRevenue),

			nm.ErrorCalls, nm.UnrecognizedCalls, nm.DownloadCalls,
			nm.RenewCalls, nm.ReviseCalls, nm.SettingsCalls,
			nm.FormContractCalls)
	} else {
		fmt.Printf(`Host info:
	Storage:      %v (%v used)
	Price:        %v / TB / Month
	Max Duration: %v Weeks

	Accepting Contracts: %v
	Anticipated Revenue: %v
	Locked Collateral:   %v
	Revenue:             %v
`,
			filesizeUnits(int64(totalstorage)),
			filesizeUnits(int64(totalstorage-storageremaining)), price,
			periodUnits(is.MaxDuration),

			yesNo(is.AcceptingContracts), currencyUnits(totalPotentialRevenue),
			currencyUnits(fm.LockedStorageCollateral),
			currencyUnits(totalRevenue))
	}

	fmt.Println("\nStorage Folders:")

	// display storage folder info
	if len(sg.StorageFolderMetadata) == 0 {
		fmt.Println("No storage folders configured")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	fmt.Fprintf(w, "\tUsed\tCapacity\t%% Used\tPath\n")
	for _, folder := range sg.StorageFolderMetadata {
		curSize := int64(folder.Capacity - folder.CapacityRemaining)
		pctUsed := 100 * (float64(curSize) / float64(folder.Capacity))
		fmt.Fprintf(w, "\t%s\t%s\t%.2f\t%s\n", filesizeUnits(curSize), filesizeUnits(int64(folder.Capacity)), pctUsed, folder.Path)
	}
	w.Flush()
}

// hostconfigcmd is the handler for the command `siac host config [setting] [value]`.
// Modifies host settings.
func hostconfigcmd(param, value string) {
	switch param {
	// currency (convert to hastings)
	case "collateralbudget", "maxcollateral", "mincontractprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		value = hastings

	// currency/TB (convert to hastings/byte)
	case "collateral", "mindownloadbandwidthprice", "minuploadbandwidthprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		i, _ := new(big.Int).SetString(hastings, 10)
		c := types.NewCurrency(i).Div(modules.BytesPerTerabyte)
		value = c.String()

	// currency/TB/month (convert to hastings/byte/block)
	case "minstorageprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		i, _ := new(big.Int).SetString(hastings, 10)
		c := types.NewCurrency(i).Div(modules.BlockBytesPerMonthTerabyte)
		value = c.String()

	// other valid settings
	case "acceptingcontracts", "maxdownloadbatchsize", "maxduration",
		"maxrevisebatchsize", "netaddress", "windowsize":

	// invalid settings
	default:
		die("\"" + param + "\" is not a host setting")
	}
	err := post("/host", param+"="+value)
	if err != nil {
		die("Could not update host settings:", err)
	}
	fmt.Println("Host settings updated.")
}

// hostannouncecmd is the handler for the command `siac host announce`.
// Announces yourself as a host to the network. Optionally takes an address to
// announce as.
func hostannouncecmd(cmd *cobra.Command, args []string) {
	var err error
	switch len(args) {
	case 0:
		err = post("/host/announce", "")
	case 1:
		err = post("/host/announce", "netaddress="+args[0])
	default:
		cmd.Usage()
		os.Exit(exitCodeUsage)
	}
	if err != nil {
		die("Could not announce host:", err)
	}
	fmt.Println("Host announcement submitted to network.")

	// start accepting contracts
	err = post("/host", "acceptingcontracts=true")
	if err != nil {
		die("Could not configure host to accept contracts:", err)
	}
	fmt.Println(`
The host has also been configured to accept contracts.
To revert this, run:
	siac host config acceptingcontracts false
`)
}

// hostfolderaddcmd adds a folder to the host.
func hostfolderaddcmd(path, size string) {
	size, err := parseFilesize(size)
	if err != nil {
		die("Could not parse size:", err)
	}
	err = post("/storage/folders/add", fmt.Sprintf("path=%s&size=%s", abs(path), size))
	if err != nil {
		die("Could not add folder:", err)
	}
	fmt.Println("Added folder", path)
}

// hostfolderremovecmd removes a folder from the host.
func hostfolderremovecmd(path string) {
	err := post("/storage/folders/remove", "path="+abs(path))
	if err != nil {
		die("Could not remove folder:", err)
	}
	fmt.Println("Removed folder", path)
}

// hostfolderresizecmd resizes a folder in the host.
func hostfolderresizecmd(path, newsize string) {
	newsize, err := parseFilesize(newsize)
	if err != nil {
		die("Could not parse size:", err)
	}
	err = post("/storage/folders/resize", fmt.Sprintf("path=%s&newsize=%s", abs(path), newsize))
	if err != nil {
		die("Could not resize folder:", err)
	}
	fmt.Printf("Resized folder %v to %v\n", path, newsize)
}

// hostsectordeletecmd deletes a sector from the host.
func hostsectordeletecmd(root string) {
	err := post("/storage/sectors/delete/"+root, "")
	if err != nil {
		die("Could not delete sector:", err)
	}
	fmt.Println("Deleted sector", root)
}
