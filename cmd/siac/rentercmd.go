package main

// TODO: If you run siac from a non-existant directory, the abs() function does
// not handle this very gracefully.

// TODO: Currently the download command will, every iteration, go through every
// single download in the download queue until it finds the right one. This
// doesn't end up hurting too much because it's very likely that the download
// you want is earlier instead of later in the queue, but it's still overhead
// that we should replace by using an API endpoint that allows you to ask for
// the desired download immediately instead of having to search through a list.
//
// The desired download should be specified by a unique ID instead of by a path,
// since a download to the same path can appear multiple times in the download
// history. This will need to be a new return value of the download call in the
// API.

// TODO: Currently the download command always displays speeds in terms of Mbps.
// This should probably be switched to some sort of human readable bandwidth
// display, so that it adjusts units as appropriate.

// TODO: Currently, the download command displays speed based on the total
// download time of the file, instead of using a rolling average over the last
// few minutes. We should change the download speed to use a rolling average.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/NebulousLabs/errors"
	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

var (
	renterAllowanceCancelCmd = &cobra.Command{
		Use:   "cancel",
		Short: "Cancel the current allowance",
		Long:  "Cancel the current allowance, which controls how much money is spent on file contracts.",
		Run:   wrap(renterallowancecancelcmd),
	}

	renterAllowanceCmd = &cobra.Command{
		Use:   "allowance",
		Short: "View the current allowance",
		Long:  "View the current allowance, which controls how much money is spent on file contracts.",
		Run:   wrap(renterallowancecmd),
	}

	renterCmd = &cobra.Command{
		Use:   "renter",
		Short: "Perform renter actions",
		Long:  "Upload, download, rename, delete, load, or share files.",
		Run:   wrap(rentercmd),
	}

	renterContractsCmd = &cobra.Command{
		Use:   "contracts",
		Short: "View the Renter's contracts",
		Long:  "View the contracts that the Renter has formed with hosts.",
		Run:   wrap(rentercontractscmd),
	}

	renterContractsViewCmd = &cobra.Command{
		Use:   "view [contract-id]",
		Short: "View details of the specified contract",
		Long:  "View all details available of the specified contract.",
		Run:   wrap(rentercontractsviewcmd),
	}

	renterDownloadsCmd = &cobra.Command{
		Use:   "downloads",
		Short: "View the download queue",
		Long:  "View the list of files currently downloading.",
		Run:   wrap(renterdownloadscmd),
	}

	renterFilesDeleteCmd = &cobra.Command{
		Use:     "delete [path]",
		Aliases: []string{"rm"},
		Short:   "Delete a file",
		Long:    "Delete a file. Does not delete the file on disk.",
		Run:     wrap(renterfilesdeletecmd),
	}

	renterFilesDownloadCmd = &cobra.Command{
		Use:   "download [path] [destination]",
		Short: "Download a file",
		Long:  "Download a previously-uploaded file to a specified destination.",
		Run:   wrap(renterfilesdownloadcmd),
	}

	renterFilesListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the status of all files",
		Long:    "List the status of all files known to the renter on the Sia network.",
		Run:     wrap(renterfileslistcmd),
	}

	renterFilesRenameCmd = &cobra.Command{
		Use:     "rename [path] [newpath]",
		Aliases: []string{"mv"},
		Short:   "Rename a file",
		Long:    "Rename a file.",
		Run:     wrap(renterfilesrenamecmd),
	}

	renterFilesUploadCmd = &cobra.Command{
		Use:   "upload [source] [path]",
		Short: "Upload a file",
		Long:  "Upload a file to [path] on the Sia network.",
		Run:   wrap(renterfilesuploadcmd),
	}

	renterPricesCmd = &cobra.Command{
		Use:   "prices",
		Short: "Display the price of storage and bandwidth",
		Long:  "Display the estimated prices of storing files, retrieving files, and creating a set of contracts",
		Run:   wrap(renterpricescmd),
	}

	renterSetAllowanceCmd = &cobra.Command{
		Use:   "setallowance [amount] [period] [hosts] [renew window]",
		Short: "Set the allowance",
		Long: `Set the amount of money that can be spent over a given period.

amount is given in currency units (SC, KS, etc.)

period is given in either blocks (b), hours (h), days (d), or weeks (w). A
block is approximately 10 minutes, so one hour is six blocks, a day is 144
blocks, and a week is 1008 blocks.

The Sia renter module spreads data across more than one Sia server computer
or "host". The "hosts" parameter for the setallowance command determines
how many different hosts the renter will spread the data across.

Allowance can be automatically renewed periodically. If the current
blockheight + the renew window >= the end height the contract,
then the contract is renewed automatically.

Note that setting the allowance will cause siad to immediately begin forming
contracts! You should only set the allowance once you are fully synced and you
have a reasonable number (>30) of hosts in your hostdb.`,
		Run: rentersetallowancecmd,
	}

	renterUploadsCmd = &cobra.Command{
		Use:   "uploads",
		Short: "View the upload queue",
		Long:  "View the list of files currently uploading.",
		Run:   wrap(renteruploadscmd),
	}
)

// abs returns the absolute representation of a path.
// TODO: bad things can happen if you run siac from a non-existent directory.
// Implement some checks to catch this problem.
func abs(path string) string {
	abspath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abspath
}

// rentercmd displays the renter's financial metrics and lists the files it is
// tracking.
func rentercmd() {
	rg, err := httpClient.RenterGet()
	if err != nil {
		die("Could not get renter info:", err)
	}
	fm := rg.FinancialMetrics
	totalSpent := fm.ContractFees.Add(fm.UploadSpending).
		Add(fm.DownloadSpending).Add(fm.StorageSpending)
	// Calculate unspent allocated
	unspentAllocated := types.ZeroCurrency
	if fm.TotalAllocated.Cmp(totalSpent) >= 0 {
		unspentAllocated = fm.TotalAllocated.Sub(totalSpent)
	}
	// Calculate unspent unallocated
	unspentUnallocated := types.ZeroCurrency
	if fm.Unspent.Cmp(unspentAllocated) >= 0 {
		unspentUnallocated = fm.Unspent.Sub(unspentAllocated)
	}

	fmt.Printf(`Renter info:
	Allowance:         %v
	Period Spending:
	  Spent Funds:     %v
	    Storage:       %v
	    Upload:        %v
	    Download:      %v
	    Fees:          %v
	  Unspent Funds:   %v
	    Allocated:     %v
	    Unallocated:   %v
	Previous Spending:
	  Withheld Funds:  %v
	  Release Block:   %v
	  Spent Funds:	   %v

`, currencyUnits(rg.Settings.Allowance.Funds), currencyUnits(totalSpent),
		currencyUnits(fm.StorageSpending), currencyUnits(fm.UploadSpending),
		currencyUnits(fm.DownloadSpending), currencyUnits(fm.ContractFees),
		currencyUnits(fm.Unspent), currencyUnits(unspentAllocated),
		currencyUnits(unspentUnallocated), currencyUnits(fm.WithheldFunds),
		fm.ReleaseBlock, currencyUnits(fm.PreviousSpending))

	// also list files
	renterfileslistcmd()
}

// renteruploadscmd is the handler for the command `siac renter uploads`.
// Lists files currently uploading.
func renteruploadscmd() {
	rf, err := httpClient.RenterFilesGet()
	if err != nil {
		die("Could not get upload queue:", err)
	}

	// TODO: add a --history flag to the uploads command to mirror the --history
	//       flag in the downloads command. This hasn't been done yet because the
	//       call to /renter/files includes files that have been shared with you,
	//       not just files you've uploaded.

	// Filter out files that have been uploaded.
	var filteredFiles []modules.FileInfo
	for _, fi := range rf.Files {
		if !fi.Available {
			filteredFiles = append(filteredFiles, fi)
		}
	}
	if len(filteredFiles) == 0 {
		fmt.Println("No files are uploading.")
		return
	}
	fmt.Println("Uploading", len(filteredFiles), "files:")
	for _, file := range filteredFiles {
		fmt.Printf("%13s  %s (uploading, %0.2f%%)\n", filesizeUnits(int64(file.Filesize)), file.SiaPath, file.UploadProgress)
	}
}

// renterdownloadscmd is the handler for the command `siac renter downloads`.
// Lists files currently downloading, and optionally previously downloaded
// files if the -H or --history flag is specified.
func renterdownloadscmd() {
	queue, err := httpClient.RenterDownloadsGet()
	if err != nil {
		die("Could not get download queue:", err)
	}
	// Filter out files that have been downloaded.
	var downloading []api.DownloadInfo
	for _, file := range queue.Downloads {
		if file.Received != file.Filesize {
			downloading = append(downloading, file)
		}
	}
	if len(downloading) == 0 {
		fmt.Println("No files are downloading.")
	} else {
		fmt.Println("Downloading", len(downloading), "files:")
		for _, file := range downloading {
			fmt.Printf("%s: %5.1f%% %s -> %s\n", file.StartTime.Format("Jan 02 03:04 PM"), 100*float64(file.Received)/float64(file.Filesize), file.SiaPath, file.Destination)
		}
	}
	if !renterShowHistory {
		return
	}
	fmt.Println()
	// Filter out files that are downloading.
	var downloaded []api.DownloadInfo
	for _, file := range queue.Downloads {
		if file.Received == file.Filesize {
			downloaded = append(downloaded, file)
		}
	}
	if len(downloaded) == 0 {
		fmt.Println("No files downloaded.")
	} else {
		fmt.Println("Downloaded", len(downloaded), "files:")
		for _, file := range downloaded {
			fmt.Printf("%s: %s -> %s\n", file.StartTime.Format("Jan 02 03:04 PM"), file.SiaPath, file.Destination)
		}
	}
}

// renterallowancecmd displays the current allowance.
func renterallowancecmd() {
	rg, err := httpClient.RenterGet()
	if err != nil {
		die("Could not get allowance:", err)
	}
	allowance := rg.Settings.Allowance

	// convert to SC
	fmt.Printf(`Allowance:
	Amount: %v
	Period: %v blocks
`, currencyUnits(allowance.Funds), allowance.Period)
}

// renterallowancecancelcmd cancels the current allowance.
func renterallowancecancelcmd() {
	fmt.Println(`Canceling your allowance will disable uploading new files,
repairing existing files, and renewing existing files. All files will cease
to be accessible after a short period of time.`)
again:
	fmt.Print("Do you want to continue? [y/n] ")
	var resp string
	fmt.Scanln(&resp)
	switch strings.ToLower(resp) {
	case "y", "yes":
		// continue below
	case "n", "no":
		return
	default:
		goto again
	}
	err := httpClient.RenterCancelAllowance()
	if err != nil {
		die("error canceling allowance:", err)
	}
	fmt.Println("Allowance canceled.")
}

// rentersetallowancecmd allows the user to set the allowance.
// the first two parameters, amount and period, are required.
// the second two parameters are optional:
//    hosts                 integer number of hosts
//    renewperiod           how many blocks between renewals
func rentersetallowancecmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 || len(args) > 4 {
		cmd.UsageFunc()(cmd)
		os.Exit(exitCodeUsage)
	}
	hastings, err := parseCurrency(args[0])
	if err != nil {
		die("Could not parse amount:", err)
	}
	blocks, err := parsePeriod(args[1])
	if err != nil {
		die("Could not parse period:", err)
	}
	allowance := modules.Allowance{}
	_, err = fmt.Sscan(hastings, &allowance.Funds)
	if err != nil {
		die("Could not parse amount:", err)
	}

	_, err = fmt.Sscan(blocks, &allowance.Period)
	if err != nil {
		die("Could not parse period:", err)
	}
	if len(args) > 2 {
		hosts, err := strconv.Atoi(args[2])
		if err != nil {
			die("Could not parse host count")
		}
		allowance.Hosts = uint64(hosts)
	}
	if len(args) > 3 {
		renewWindow, err := parsePeriod(args[3])
		if err != nil {
			die("Could not parse renew window")
		}
		_, err = fmt.Sscan(renewWindow, &allowance.RenewWindow)
		if err != nil {
			die("Could not parse renew window:", err)
		}
	}
	err = httpClient.RenterPostAllowance(allowance)
	if err != nil {
		die("Could not set allowance:", err)
	}
	fmt.Println("Allowance updated.")
}

// byValue sorts contracts by their value in siacoins, high to low. If two
// contracts have the same value, they are sorted by their host's address.
type byValue []api.RenterContract

func (s byValue) Len() int      { return len(s) }
func (s byValue) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byValue) Less(i, j int) bool {
	cmp := s[i].RenterFunds.Cmp(s[j].RenterFunds)
	if cmp == 0 {
		return s[i].NetAddress < s[j].NetAddress
	}
	return cmp > 0
}

// rentercontractscmd is the handler for the comand `siac renter contracts`.
// It lists the Renter's contracts.
func rentercontractscmd() {
	rc, err := httpClient.RenterInactiveContractsGet()
	if err != nil {
		die("Could not get contracts:", err)
	}
	if len(rc.ActiveContracts) == 0 && len(rc.InactiveContracts) == 0 && !renterAllContracts {
		fmt.Println("No contracts in the current period.")
		return
	}

	if len(rc.ActiveContracts) != 0 && len(rc.InactiveContracts) != 0 {
		// Display Active Contracts
		fmt.Println("Contracts in the Current Period")
		sort.Sort(byValue(rc.ActiveContracts))
		var activeTotalStored uint64
		var activeTotalRemaining, activeTotalSpent, activeTotalFees types.Currency
		for _, c := range rc.ActiveContracts {
			activeTotalStored += c.Size
			activeTotalRemaining = activeTotalRemaining.Add(c.RenterFunds)
			activeTotalSpent = activeTotalSpent.Add(c.TotalCost.Sub(c.RenterFunds).Sub(c.Fees))
			activeTotalFees = activeTotalFees.Add(c.Fees)
		}
		fmt.Printf(`
Active Contract Summary:
Number of Contracts:  %v
Total stored:         %9s
Total Remaining:      %v
Total Spent:          %v
Total Fees:           %v

`, len(rc.ActiveContracts), filesizeUnits(int64(activeTotalStored)), currencyUnits(activeTotalRemaining), currencyUnits(activeTotalSpent), currencyUnits(activeTotalFees))
		w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Host\tRemaining Funds\tSpent Funds\tSpent Fees\tData\tEnd Height\tID\tGoodForUpload\tGoodForRenew")
		for _, c := range rc.ActiveContracts {
			address := c.NetAddress
			if address == "" {
				address = "Host Removed"
			}
			fmt.Fprintf(w, "%v\t%8s\t%8s\t%8s\t%v\t%v\t%v\t%v\t%v\n",
				address,
				currencyUnits(c.RenterFunds),
				currencyUnits(c.TotalCost.Sub(c.RenterFunds).Sub(c.Fees)),
				currencyUnits(c.Fees),
				filesizeUnits(int64(c.Size)),
				c.EndHeight,
				c.ID,
				c.GoodForUpload,
				c.GoodForRenew)
		}
		w.Flush()

		// Display Inactive Contracts
		sort.Sort(byValue(rc.InactiveContracts))
		var inactiveTotalStored uint64
		var inactiveTotalRemaining, inactiveTotalSpent, inactiveTotalFees types.Currency
		for _, c := range rc.InactiveContracts {
			inactiveTotalStored += c.Size
			inactiveTotalRemaining = inactiveTotalRemaining.Add(c.RenterFunds)
			inactiveTotalSpent = inactiveTotalSpent.Add(c.TotalCost.Sub(c.RenterFunds).Sub(c.Fees))
			inactiveTotalFees = inactiveTotalFees.Add(c.Fees)
		}
		fmt.Printf(`
Inactive Contract Summary:
Number of Contracts:  %v
Total stored:         %9s
Total Remaining:      %v
Total Spent:          %v
Total Fees:           %v
			
`, len(rc.InactiveContracts), filesizeUnits(int64(inactiveTotalStored)), currencyUnits(inactiveTotalRemaining), currencyUnits(inactiveTotalSpent), currencyUnits(inactiveTotalFees))
		w = tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Host\tRemaining Funds\tSpent Funds\tSpent Fees\tData\tEnd Height\tID\tGoodForUpload\tGoodForRenew")
		for _, c := range rc.InactiveContracts {
			address := c.NetAddress
			if address == "" {
				address = "Host Removed"
			}
			fmt.Fprintf(w, "%v\t%8s\t%8s\t%8s\t%v\t%v\t%v\t%v\t%v\n",
				address,
				currencyUnits(c.RenterFunds),
				currencyUnits(c.TotalCost.Sub(c.RenterFunds).Sub(c.Fees)),
				currencyUnits(c.Fees),
				filesizeUnits(int64(c.Size)),
				c.EndHeight,
				c.ID,
				c.GoodForUpload,
				c.GoodForRenew)
		}
		w.Flush()
	}

	if renterAllContracts {
		rce, err := httpClient.RenterExpiredContractsGet()
		if err != nil {
			die("Could not get expired contracts:", err)
		}
		if len(rc.ActiveContracts) == 0 && len(rc.InactiveContracts) == 0 && len(rce.ExpiredContracts) == 0 {
			fmt.Println("No contracts have been formed.")
			return
		}
		if len(rce.ExpiredContracts) == 0 {
			fmt.Println("No expired contracts")
			return
		}
		sort.Sort(byValue(rce.ExpiredContracts))
		var expiredTotalStored uint64
		var expiredTotalWithheld, expiredTotalSpent, expiredTotalFees types.Currency
		for _, c := range rce.ExpiredContracts {
			expiredTotalStored += c.Size
			expiredTotalWithheld = expiredTotalWithheld.Add(c.RenterFunds)
			expiredTotalSpent = expiredTotalSpent.Add(c.TotalCost.Sub(c.RenterFunds).Sub(c.Fees))
			expiredTotalFees = expiredTotalFees.Add(c.Fees)
		}
		fmt.Printf(`
Expired Contract Summary:
Number of Contracts:  %v
Total stored:         %9s
Total Remaining:      %v
Total Spent:          %v
Total Fees:           %v
		
`, len(rce.ExpiredContracts), filesizeUnits(int64(expiredTotalStored)), currencyUnits(expiredTotalWithheld), currencyUnits(expiredTotalSpent), currencyUnits(expiredTotalFees))
		w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Host\tWithheld Funds\tSpent Funds\tSpent Fees\tData\tEnd Height\tID\tGoodForUpload\tGoodForRenew")
		for _, c := range rce.ExpiredContracts {
			address := c.NetAddress
			if address == "" {
				address = "Host Removed"
			}
			fmt.Fprintf(w, "%v\t%8s\t%8s\t%8s\t%v\t%v\t%v\t%v\t%v\n",
				address,
				currencyUnits(c.RenterFunds),
				currencyUnits(c.TotalCost.Sub(c.RenterFunds).Sub(c.Fees)),
				currencyUnits(c.Fees),
				filesizeUnits(int64(c.Size)),
				c.EndHeight,
				c.ID,
				c.GoodForUpload,
				c.GoodForRenew)
		}
		w.Flush()
	}
}

// rentercontractsviewcmd is the handler for the command `siac renter contracts <id>`.
// It lists details of a specific contract.
func rentercontractsviewcmd(cid string) {
	rc, err := httpClient.RenterInactiveContractsGet()
	if err != nil {
		die("Could not get contract details: ", err)
	}
	rce, err := httpClient.RenterExpiredContractsGet()
	if err != nil {
		die("Could not get expired contract details: ", err)
	}

	contracts := append(rc.ActiveContracts, rc.InactiveContracts...)
	contracts = append(contracts, rce.ExpiredContracts...)

	for _, rc := range contracts {
		if rc.ID.String() == cid {
			hostInfo, err := httpClient.HostDbHostsGet(rc.HostPublicKey)
			if err != nil {
				die("Could not fetch details of host: ", err)
			}
			fmt.Printf(`
Contract %v
  Host: %v (Public Key: %v)

  Start Height: %v
  End Height:   %v

  Total cost:        %v (Fees: %v)
  Funds Allocated:   %v
  Upload Spending:   %v
  Storage Spending:  %v
  Download Spending: %v
  Remaining Funds:   %v

  File Size: %v
`, rc.ID, rc.NetAddress, rc.HostPublicKey.String(), rc.StartHeight, rc.EndHeight,
				currencyUnits(rc.TotalCost),
				currencyUnits(rc.Fees),
				currencyUnits(rc.TotalCost.Sub(rc.Fees)),
				currencyUnits(rc.UploadSpending),
				currencyUnits(rc.StorageSpending),
				currencyUnits(rc.DownloadSpending),
				currencyUnits(rc.RenterFunds),
				filesizeUnits(int64(rc.Size)))

			printScoreBreakdown(&hostInfo)
			return
		}
	}

	fmt.Println("Contract not found")
}

// renterfilesdeletecmd is the handler for the command `siac renter delete [path]`.
// Removes the specified path from the Sia network.
func renterfilesdeletecmd(path string) {
	err := httpClient.RenterDeletePost(path)
	if err != nil {
		die("Could not delete file:", err)
	}
	fmt.Println("Deleted", path)
}

// renterfilesdownloadcmd is the handler for the comand `siac renter download [path] [destination]`.
// Downloads a path from the Sia network to the local specified destination.
func renterfilesdownloadcmd(path, destination string) {
	destination = abs(destination)

	// Queue the download. An error will be returned if the queueing failed, but
	// the call will return before the download has completed. The call is made
	// as an async call.
	err := httpClient.RenterDownloadFullGet(path, destination, true)
	if err != nil {
		die("Download could not be started:", err)
	}

	// If the download is async, report success.
	if renterDownloadAsync {
		fmt.Printf("Queued Download '%s' to %s.\n", path, abs(destination))
		return
	}

	// If the download is blocking, display progress as the file downloads.
	err = downloadprogress(path, destination)
	if err != nil {
		die("\nDownload could not be completed:", err)
	}
	fmt.Printf("\nDownloaded '%s' to %s.\n", path, abs(destination))
}

// downloadprogress will display the progress of the provided download to the
// user, and return an error when the download is finished.
func downloadprogress(siapath, destination string) error {
	start := time.Now()
	for range time.Tick(OutputRefreshRate) {
		// Get the list of downloads.
		queue, err := httpClient.RenterDownloadsGet()
		if err != nil {
			continue // benign
		}

		// Search for the download in the list of downloads.
		var d api.DownloadInfo
		found := false
		for _, d = range queue.Downloads {
			if d.SiaPath == siapath && d.Destination == destination {
				found = true
				break
			}
		}
		// If the download has not appeared in the queue yet, either continue or
		// give up.
		if !found {
			if time.Since(start) > RenterDownloadTimeout {
				return errors.New("Unable to find download in queue")
			}
			continue
		}

		// Check whether the file has completed or otherwise errored out.
		if d.Error != "" {
			return errors.New(d.Error)
		}
		if d.Completed {
			return nil
		}

		// Update the progress for the user.
		pct := 100 * float64(d.Received) / float64(d.Filesize)
		elapsed := time.Since(d.StartTime)
		elapsed -= elapsed % time.Second // round to nearest second
		mbps := (float64(d.Received*8) / 1e6) / time.Since(d.StartTime).Seconds()
		fmt.Printf("\rDownloading... %5.1f%% of %v, %v elapsed, %.2f Mbps    ", pct, filesizeUnits(int64(d.Filesize)), elapsed, mbps)
	}

	// This code is unreachable, but the complier requires this to be here.
	return errors.New("ERROR: download progress reached code that should not be reachable.")
}

// bySiaPath implements sort.Interface for [] modules.FileInfo based on the
// SiaPath field.
type bySiaPath []modules.FileInfo

func (s bySiaPath) Len() int           { return len(s) }
func (s bySiaPath) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s bySiaPath) Less(i, j int) bool { return s[i].SiaPath < s[j].SiaPath }

// renterfileslistcmd is the handler for the command `siac renter list`.
// Lists files known to the renter on the network.
func renterfileslistcmd() {
	var rf api.RenterFiles
	rf, err := httpClient.RenterFilesGet()
	if err != nil {
		die("Could not get file list:", err)
	}
	if len(rf.Files) == 0 {
		fmt.Println("No files have been uploaded.")
		return
	}
	fmt.Println("Tracking", len(rf.Files), "files:")
	var totalStored uint64
	for _, file := range rf.Files {
		totalStored += file.Filesize
	}
	fmt.Printf("Total uploaded: %9s\n", filesizeUnits(int64(totalStored)))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if renterListVerbose {
		fmt.Fprintln(w, "File size\tAvailable\tUploaded\tProgress\tRedundancy\tRenewing\tOn Disk\tRecoverable\tSia path")
	}
	sort.Sort(bySiaPath(rf.Files))
	for _, file := range rf.Files {
		fmt.Fprintf(w, "%9s", filesizeUnits(int64(file.Filesize)))
		if renterListVerbose {
			availableStr := yesNo(file.Available)
			renewingStr := yesNo(file.Renewing)
			redundancyStr := fmt.Sprintf("%.2f", file.Redundancy)
			if file.Redundancy == -1 {
				redundancyStr = "-"
			}
			uploadProgressStr := fmt.Sprintf("%.2f%%", file.UploadProgress)
			_, err := os.Stat(file.LocalPath)
			onDiskStr := yesNo(!os.IsNotExist(err))
			recoverableStr := yesNo(!(file.Redundancy < 1))
			if file.UploadProgress == -1 {
				uploadProgressStr = "-"
			}
			fmt.Fprintf(w, "\t%s\t%9s\t%8s\t%10s\t%s\t%s\t%s", availableStr, filesizeUnits(int64(file.UploadedBytes)), uploadProgressStr, redundancyStr, renewingStr, onDiskStr, recoverableStr)
		}
		fmt.Fprintf(w, "\t%s", file.SiaPath)
		if !renterListVerbose && !file.Available {
			fmt.Fprintf(w, " (uploading, %0.2f%%)", file.UploadProgress)
		}
		fmt.Fprintln(w, "")
	}
	w.Flush()
}

// renterfilesrenamecmd is the handler for the command `siac renter rename [path] [newpath]`.
// Renames a file on the Sia network.
func renterfilesrenamecmd(path, newpath string) {
	err := httpClient.RenterRenamePost(path, newpath)
	if err != nil {
		die("Could not rename file:", err)
	}
	fmt.Printf("Renamed %s to %s\n", path, newpath)
}

// renterfilesuploadcmd is the handler for the command `siac renter upload
// [source] [path]`. Uploads the [source] file to [path] on the Sia network.
// If [source] is a directory, all files inside it will be uploaded and named
// relative to [path].
func renterfilesuploadcmd(source, path string) {
	stat, err := os.Stat(source)
	if err != nil {
		die("Could not stat file or folder:", err)
	}

	if stat.IsDir() {
		// folder
		var files []string
		err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Println("Warning: skipping file:", err)
				return nil
			}
			if info.IsDir() {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			die("Could not read folder:", err)
		} else if len(files) == 0 {
			die("Nothing to upload.")
		}
		for _, file := range files {
			fpath, _ := filepath.Rel(source, file)
			fpath = filepath.Join(path, fpath)
			fpath = filepath.ToSlash(fpath)
			err = httpClient.RenterUploadDefaultPost(abs(file), fpath)
			if err != nil {
				die("Could not upload file:", err)
			}
		}
		fmt.Printf("Uploaded %d files into '%s'.\n", len(files), path)
	} else {
		// single file
		err = httpClient.RenterUploadDefaultPost(abs(source), path)
		if err != nil {
			die("Could not upload file:", err)
		}
		fmt.Printf("Uploaded '%s' as %s.\n", abs(source), path)
	}
}

// renterpricescmd is the handler for the command `siac renter prices`, which
// displays the prices of various storage operations.
func renterpricescmd() {
	rpg, err := httpClient.RenterPricesGet()
	if err != nil {
		die("Could not read the renter prices:", err)
	}

	fmt.Println("Renter Prices (estimated):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tFees for Creating a Set of Contracts:\t", currencyUnits(rpg.FormContracts))
	fmt.Fprintln(w, "\tDownload 1 TB:\t", currencyUnits(rpg.DownloadTerabyte))
	fmt.Fprintln(w, "\tStore 1 TB for 1 Month:\t", currencyUnits(rpg.StorageTerabyteMonth))
	fmt.Fprintln(w, "\tUpload 1 TB:\t", currencyUnits(rpg.UploadTerabyte))
	w.Flush()
}
