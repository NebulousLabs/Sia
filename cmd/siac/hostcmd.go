package main

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node/api/client"
	"github.com/NebulousLabs/Sia/types"

	"github.com/spf13/cobra"
)

var (
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
     acceptingcontracts:   boolean
     maxduration:          blocks
     maxdownloadbatchsize: bytes
     maxrevisebatchsize:   bytes
     netaddress:           string
     windowsize:           blocks

     collateral:       currency
     collateralbudget: currency
     maxcollateral:    currency

     mincontractprice:          currency
     mindownloadbandwidthprice: currency / TB
     minstorageprice:           currency / TB / Month
     minuploadbandwidthprice:   currency / TB

Currency units can be specified, e.g. 10SC; run 'siac help wallet' for details.

Durations (maxduration and windowsize) must be specified in either blocks (b),
hours (h), days (d), or weeks (w). A block is approximately 10 minutes, so one
hour is six blocks, a day is 144 blocks, and a week is 1008 blocks.

For a description of each parameter, see doc/API.md.

To configure the host to accept new contracts, set acceptingcontracts to true:
	siac host config acceptingcontracts true
`,
		Run: wrap(hostconfigcmd),
	}

	hostContractCmd = &cobra.Command{
		Use:   "contracts",
		Short: "Show host contracts",
		Long: `Show host contracts sorted by expiration height.

Available output types:
     value:  show financial information
     status: show status information
`,
		Run: wrap(hostcontractcmd),
	}

	hostFolderAddCmd = &cobra.Command{
		Use:   "add [path] [size]",
		Short: "Add a storage folder to the host",
		Long:  "Add a storage folder to the host, specifying how much data it should store",
		Run:   wrap(hostfolderaddcmd),
	}

	hostFolderCmd = &cobra.Command{
		Use:   "folder",
		Short: "Add, remove, or resize a storage folder",
		Long:  "Add, remove, or resize a storage folder.",
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
	hg, err := httpClient.HostGet()
	if err != nil {
		die("Could not fetch host settings:", err)
	}
	sg, err := httpClient.HostStorageGet()
	if err != nil {
		die("Could not fetch storage info:", err)
	}

	es := hg.ExternalSettings
	fm := hg.FinancialMetrics
	is := hg.InternalSettings
	nm := hg.NetworkMetrics

	// calculate total storage available and remaining
	var totalstorage, storageremaining uint64
	for _, folder := range sg.Folders {
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

	var connectabilityString string
	if hg.WorkingStatus == "working" {
		connectabilityString = "Host appears to be working."
	} else if hg.WorkingStatus == "not working" && hg.ConnectabilityStatus == "connectable" {
		connectabilityString = "Nobody is connecting to host. Try re-announcing."
	} else if hg.WorkingStatus == "checking" || hg.ConnectabilityStatus == "checking" {
		connectabilityString = "Host is checking status (takes a few minues)."
	} else {
		connectabilityString = "Host is not connectable (re-checks every few minutes)."
	}

	if hostVerbose {
		// describe net address
		fmt.Printf(`General Info:
	Connectability Status: %v

Host Internal Settings:
	acceptingcontracts:   %v
	maxduration:          %v Weeks
	maxdownloadbatchsize: %v
	maxrevisebatchsize:   %v
	netaddress:           %v
	windowsize:           %v Hours

	collateral:       %v / TB / Month
	collateralbudget: %v
	maxcollateral:    %v Per Contract

	mincontractprice:          %v
	mindownloadbandwidthprice: %v / TB
	minstorageprice:           %v / TB / Month
	minuploadbandwidthprice:   %v / TB

Host Financials:
	Contract Count:               %v
	Transaction Fee Compensation: %v
	Potential Fee Compensation:   %v
	Transaction Fee Expenses:     %v

	Storage Revenue:           %v
	Potential Storage Revenue: %v

	Locked Collateral: %v
	Risked Collateral: %v
	Lost Collateral:   %v

	Download Revenue:           %v
	Potential Download Revenue: %v
	Upload Revenue:             %v
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
			connectabilityString,

			yesNo(is.AcceptingContracts), periodUnits(is.MaxDuration),
			filesizeUnits(int64(is.MaxDownloadBatchSize)),
			filesizeUnits(int64(is.MaxReviseBatchSize)), netaddr,
			is.WindowSize/6,

			currencyUnits(is.Collateral.Mul(modules.BlockBytesPerMonthTerabyte)),
			currencyUnits(is.CollateralBudget),
			currencyUnits(is.MaxCollateral),

			currencyUnits(is.MinContractPrice),
			currencyUnits(is.MinDownloadBandwidthPrice.Mul(modules.BytesPerTerabyte)),
			currencyUnits(is.MinStoragePrice.Mul(modules.BlockBytesPerMonthTerabyte)),
			currencyUnits(is.MinUploadBandwidthPrice.Mul(modules.BytesPerTerabyte)),

			fm.ContractCount, currencyUnits(fm.ContractCompensation),
			currencyUnits(fm.PotentialContractCompensation),
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
	Connectability Status: %v

	Storage:      %v (%v used)
	Price:        %v / TB / Month
	Max Duration: %v Weeks

	Accepting Contracts:  %v
	Anticipated Revenue:  %v
	Locked Collateral:    %v
	Revenue:              %v
`,
			connectabilityString,

			filesizeUnits(int64(totalstorage)),
			filesizeUnits(int64(totalstorage-storageremaining)), price,
			periodUnits(is.MaxDuration),

			yesNo(is.AcceptingContracts), currencyUnits(totalPotentialRevenue),
			currencyUnits(fm.LockedStorageCollateral),
			currencyUnits(totalRevenue))
	}

	// if wallet is locked print warning
	walletstatus, walleterr := httpClient.WalletGet()
	if walleterr != nil {
		fmt.Print("\nWarning:\n	Could not get wallet status. A working wallet is needed in order to operate your host. Error: ")
		fmt.Println(walleterr)
	} else if !walletstatus.Unlocked {
		fmt.Println("\nWarning:\n	Your wallet is locked. You must unlock your wallet for the host to function properly.")
	}

	fmt.Println("\nStorage Folders:")

	// display storage folder info
	sort.Slice(sg.Folders, func(i, j int) bool {
		return sg.Folders[i].Path < sg.Folders[j].Path
	})
	if len(sg.Folders) == 0 {
		fmt.Println("No storage folders configured")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	fmt.Fprintf(w, "\tUsed\tCapacity\t%% Used\tPath\n")
	for _, folder := range sg.Folders {
		curSize := int64(folder.Capacity - folder.CapacityRemaining)
		pctUsed := 100 * (float64(curSize) / float64(folder.Capacity))
		fmt.Fprintf(w, "\t%s\t%s\t%.2f\t%s\n", filesizeUnits(curSize), filesizeUnits(int64(folder.Capacity)), pctUsed, folder.Path)
	}
	w.Flush()
}

// hostconfigcmd is the handler for the command `siac host config [setting] [value]`.
// Modifies host settings.
func hostconfigcmd(param, value string) {
	var err error
	switch param {
	// currency (convert to hastings)
	case "collateralbudget", "maxcollateral", "mincontractprice":
		value, err = parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}

	// currency/TB (convert to hastings/byte)
	case "mindownloadbandwidthprice", "minuploadbandwidthprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		i, _ := new(big.Int).SetString(hastings, 10)
		c := types.NewCurrency(i).Div(modules.BytesPerTerabyte)
		value = c.String()

	// currency/TB/month (convert to hastings/byte/block)
	case "collateral", "minstorageprice":
		hastings, err := parseCurrency(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}
		i, _ := new(big.Int).SetString(hastings, 10)
		c := types.NewCurrency(i).Div(modules.BlockBytesPerMonthTerabyte)
		value = c.String()

	// bool (allow "yes" and "no")
	case "acceptingcontracts":
		switch strings.ToLower(value) {
		case "yes":
			value = "true"
		case "no":
			value = "false"
		}

	// duration (convert to blocks)
	case "maxduration", "windowsize":
		value, err = parsePeriod(value)
		if err != nil {
			die("Could not parse "+param+":", err)
		}

	// other valid settings
	case "maxdownloadbatchsize", "maxrevisebatchsize", "netaddress":

	// invalid settings
	default:
		die("\"" + param + "\" is not a host setting")
	}
	err = httpClient.HostModifySettingPost(client.HostParam(param), value)
	if err != nil {
		die("Failed to update host settings:", err)
	}
	fmt.Println("Host settings updated.")

	// get the estimated conversion rate.
	eg, err := httpClient.HostEstimateScoreGet(param, value)
	if err != nil {
		if err.Error() == "cannot call /host/estimatescore without the renter module" {
			// score estimate requires the renter module
			return
		}
		die("could not get host score estimate:", err)
	}
	fmt.Printf("Estimated conversion rate: %v%%\n", eg.ConversionRate)
}

// hostcontractcmd is the handler for the command `siac host contracts [type]`.
func hostcontractcmd() {
	cg, err := httpClient.HostContractInfoGet()
	if err != nil {
		die("Could not fetch host contract info:", err)
	}
	sort.Slice(cg.Contracts, func(i, j int) bool { return cg.Contracts[i].ExpirationHeight < cg.Contracts[j].ExpirationHeight })
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	switch hostContractOutputType {
	case "value":
		fmt.Fprintf(w, "Obligation Id\tObligation Status\tContract Cost\tLocked Collateral\tRisked Collateral\tPotential Revenue\tExpiration Height\tTransaction Fees\n")
		for _, so := range cg.Contracts {
			potentialRevenue := so.PotentialDownloadRevenue.Add(so.PotentialUploadRevenue).Add(so.PotentialStorageRevenue)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", so.ObligationId, strings.TrimPrefix(so.ObligationStatus, "obligation"), currencyUnits(so.ContractCost), currencyUnits(so.LockedCollateral),
				currencyUnits(so.RiskedCollateral), currencyUnits(potentialRevenue), so.ExpirationHeight, currencyUnits(so.TransactionFeesAdded))
		}
	case "status":
		fmt.Fprintf(w, "Obligation ID\tObligation Status\tExpiration Height\tOrigin Confirmed\tRevision Constructed\tRevision Confirmed\tProof Constructed\tProof Confirmed\n")
		for _, so := range cg.Contracts {
			fmt.Fprintf(w, "%s\t%s\t%d\t%t\t%t\t%t\t%t\t%t\n", so.ObligationId, strings.TrimPrefix(so.ObligationStatus, "obligation"), so.ExpirationHeight, so.OriginConfirmed,
				so.RevisionConstructed, so.RevisionConfirmed, so.ProofConstructed, so.ProofConfirmed)
		}
	default:
		die("\"" + hostContractOutputType + "\" is not a format")
	}
	w.Flush()
}

// hostannouncecmd is the handler for the command `siac host announce`.
// Announces yourself as a host to the network. Optionally takes an address to
// announce as.
func hostannouncecmd(cmd *cobra.Command, args []string) {
	var err error
	switch len(args) {
	case 0:
		err = httpClient.HostAnnouncePost()
	case 1:
		err = httpClient.HostAnnounceAddrPost(modules.NetAddress(args[0]))
	default:
		cmd.UsageFunc()(cmd)
		os.Exit(exitCodeUsage)
	}
	if err != nil {
		die("Could not announce host:", err)
	}
	fmt.Println("Host announcement submitted to network.")

	// start accepting contracts
	err = httpClient.HostModifySettingPost(client.HostParamAcceptingContracts, true)
	if err != nil {
		die("Could not configure host to accept contracts:", err)
	}
	fmt.Println(`The host has also been configured to accept contracts.
To revert this, run:
	siac host config acceptingcontracts false`)
}

// hostfolderaddcmd adds a folder to the host.
func hostfolderaddcmd(path, size string) {
	size, err := parseFilesize(size)
	if err != nil {
		die("Could not parse size:", err)
	}
	// round size down to nearest multiple of 256MiB
	var sizeUint64 uint64
	fmt.Sscan(size, &sizeUint64)
	sizeUint64 /= 64 * modules.SectorSize
	sizeUint64 *= 64 * modules.SectorSize

	err = httpClient.HostStorageFoldersAddPost(abs(path), sizeUint64)
	if err != nil {
		die("Could not add folder:", err)
	}
	fmt.Println("Added folder", path)
}

// hostfolderremovecmd removes a folder from the host.
func hostfolderremovecmd(path string) {
	err := httpClient.HostStorageFoldersRemovePost(abs(path))
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
	// round size down to nearest multiple of 256MiB
	var sizeUint64 uint64
	fmt.Sscan(newsize, &sizeUint64)
	sizeUint64 /= 64 * modules.SectorSize
	sizeUint64 *= 64 * modules.SectorSize

	err = httpClient.HostStorageFoldersResizePost(abs(path), sizeUint64)
	if err != nil {
		die("Could not resize folder:", err)
	}
	fmt.Printf("Resized folder %v to %v\n", path, newsize)
}

// hostsectordeletecmd deletes a sector from the host.
func hostsectordeletecmd(root string) {
	var hash crypto.Hash
	err := hash.LoadString(root)
	if err != nil {
		die("Could not parse root:", err)
	}
	err = httpClient.HostStorageSectorsDeletePost(hash)
	if err != nil {
		die("Could not delete sector:", err)
	}
	fmt.Println("Deleted sector", root)
}
