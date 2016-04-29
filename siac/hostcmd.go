package main

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"

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
minimumcontractprice             currency
minimumdownloadbandwidthprice    currency/TB
minimumstorageprice              currency/TB/month
minimumuploadbandwidthprice      currency/TB
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

	// convert accepting bool
	accept := yesNo(is.AcceptingContracts)
	// convert price to SC/TB/mo
	price, err := modules.StoragePriceToHuman(is.MinimumStoragePrice)
	if err != nil {
		price = ^uint64(0)
	}
	// calculate total revenue
	totalRevenue := fm.ContractCompensation.
		Add(fm.StorageRevenue).
		Add(fm.DownloadBandwidthRevenue).
		Add(fm.UploadBandwidthRevenue)
	totalPotentialRevenue := fm.PotentialContractCompensation.
		Add(fm.PotentialStorageRevenue).
		Add(fm.PotentialDownloadBandwidthRevenue).
		Add(fm.PotentialUploadBandwidthRevenue)
	fmt.Printf(`Host info:
	Storage:      %v (%v used)
	Price:        %v SC per TB per month
	Max Duration: %v Blocks

	Accepting Contracts: %v
	Anticipated Revenue: %v
	Revenue:             %v
	Lost Revenue:        %v
	Lost Collateral:     %v
`, filesizeUnits(int64(totalstorage)), filesizeUnits(int64(totalstorage-storageremaining)),
		price, is.MaxDuration, accept, currencyUnits(totalPotentialRevenue),
		currencyUnits(totalRevenue), currencyUnits(fm.LostRevenue),
		currencyUnits(fm.LostStorageCollateral))

	// display more info if verbose flag is set
	if hostVerbose {
		// describe net address
		netaddr := es.NetAddress
		if is.NetAddress == "" {
			netaddr += " (automatically determined)"
		} else {
			netaddr += " (manually specified)"
		}
		fmt.Printf(`
	Net Address: %v

RPC Stats:
	Error Calls:        %v
	Unrecognized Calls: %v
	Download Calls:     %v
	Renew Calls:        %v
	Revise Calls:       %v
	Settings Calls:     %v
	FormContract Calls: %v
`, netaddr, nm.ErrorCalls, nm.UnrecognizedCalls, nm.DownloadCalls,
			nm.RenewCalls, nm.ReviseCalls, nm.SettingsCalls, nm.FormContractCalls)
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
	case "collateralbudget", "maxcollateral", "minimumcontractprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		value = hastings

	// currency/TB (convert to hastings/byte)
	case "collateral", "minimumdownloadbandwidthprice", "minimumuploadbandwidthprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		i, _ := new(big.Int).SetString(hastings, 10)
		i.Div(i, big.NewInt(1e12)) // divide by 1e12 bytes/TB
		value = i.String()

	// currency/TB/month (convert to hastings/byte/block)
	case "minimumstorageprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		i, _ := new(big.Int).SetString(hastings, 10)
		i.Div(i, big.NewInt(1e12)) // divide by 1e12 bytes/TB
		value = i.String()

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
}

// hostfolderaddcmd adds a folder to the host.
func hostfolderaddcmd(path, size string) {
	size, err := parseFilesize(size)
	if err != nil {
		die("Could not parse size:", err)
	}
	err = post("/storage/folders/add"+filepath.ToSlash(abs(path)), "size="+size)
	if err != nil {
		die("Could not add folder:", err)
	}
	fmt.Println("Added folder", path)
}

// hostfolderremovecmd removes a folder from the host.
func hostfolderremovecmd(path string) {
	err := post("/storage/folders/remove"+filepath.ToSlash(abs(path)), "")
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
	err = post("/storage/folders/resize"+filepath.ToSlash(abs(path)), "newsize="+newsize)
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
