package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	renterCmd = &cobra.Command{
		Use:   "renter",
		Short: "Perform renter actions",
		Long:  "Upload, download, rename, delete, load, or share files.",
		Run:   wrap(rentercmd),
	}

	renterUploadsCmd = &cobra.Command{
		Use:   "uploads",
		Short: "View the upload queue",
		Long:  "View the list of files currently uploading.",
		Run:   wrap(renteruploadscmd),
	}

	renterDownloadsCmd = &cobra.Command{
		Use:   "downloads",
		Short: "View the download queue",
		Long:  "View the list of files currently downloading.",
		Run:   wrap(renterdownloadscmd),
	}

	renterAllowanceCmd = &cobra.Command{
		Use:   "allowance",
		Short: "View the current allowance",
		Long:  "View the current allowance, which controls how much money is spent on file contracts.",
		Run:   wrap(renterallowancecmd),
	}
	renterSetAllowanceCmd = &cobra.Command{
		Use:   "setallowance [amount] [period]",
		Short: "Set the allowance",
		Long: `Set the amount of money that can be spent over a given period.
amount is given in currency units (SC, KS, etc.)
period is given in weeks; 1 week is roughly 1000 blocks

Note that setting the allowance will cause siad to immediately begin forming
contracts! You should only set the allowance once you are fully synced and you
have a reasonable number (>30) of hosts in your hostdb.`,
		Run: wrap(rentersetallowancecmd),
	}

	renterContractsCmd = &cobra.Command{
		Use:   "contracts",
		Short: "View the Renter's contracts",
		Long:  "View the contracts that the Renter has formed with hosts.",
		Run:   wrap(rentercontractscmd),
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
	var rg api.RenterGET
	err := getAPI("/renter", &rg)
	if err != nil {
		die("Could not get renter info:", err)
	}
	fm := rg.FinancialMetrics
	unspent := fm.ContractSpending.Sub(fm.DownloadSpending).Sub(fm.StorageSpending).Sub(fm.UploadSpending)
	fmt.Printf(`Renter info:
	Storage Spending:  %v
	Upload Spending:   %v
	Download Spending: %v
	Unspent Funds:     %v
	Total Allocated:   %v

`, currencyUnits(fm.StorageSpending), currencyUnits(fm.UploadSpending),
		currencyUnits(fm.DownloadSpending), currencyUnits(unspent),
		currencyUnits(fm.ContractSpending))

	// also list files
	renterfileslistcmd()
}

// renteruploadscmd is the handler for the command `siac renter uploads`.
// Lists files currently uploading.
func renteruploadscmd() {
	var rf api.RenterFiles
	err := getAPI("/renter/files", &rf)
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
	var queue api.RenterDownloadQueue
	err := getAPI("/renter/downloads", &queue)
	if err != nil {
		die("Could not get download queue:", err)
	}
	// Filter out files that have been downloaded.
	var downloading []modules.DownloadInfo
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
	var downloaded []modules.DownloadInfo
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
	var rg api.RenterGET
	err := getAPI("/renter", &rg)
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

// rentersetallowancecmd allows the user to set the allowance.
func rentersetallowancecmd(amount, period string) {
	hastings, err := parseCurrency(amount)
	if err != nil {
		die("Could not parse amount:", err)
	}
	blocks, err := parsePeriod(period)
	if err != nil {
		die("Could not parse period")
	}
	err = post("/renter", fmt.Sprintf("funds=%s&period=%s", hastings, blocks))
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
	var rc api.RenterContracts
	err := getAPI("/renter/contracts", &rc)
	if err != nil {
		die("Could not get contracts:", err)
	}
	if len(rc.Contracts) == 0 {
		fmt.Println("No contracts have been formed.")
		return
	}
	sort.Sort(byValue(rc.Contracts))
	fmt.Println("Contracts:")
	w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Host\tValue\tData\tEnd Height\tID")
	for _, c := range rc.Contracts {
		fmt.Fprintf(w, "%v\t%8s\t%v\t%v\t%v\n",
			c.NetAddress,
			currencyUnits(c.RenterFunds),
			filesizeUnits(int64(c.Size)),
			c.EndHeight,
			c.ID)
	}
	w.Flush()
}

// renterfilesdeletecmd is the handler for the command `siac renter delete [path]`.
// Removes the specified path from the Sia network.
func renterfilesdeletecmd(path string) {
	err := post("/renter/delete/"+path, "")
	if err != nil {
		die("Could not delete file:", err)
	}
	fmt.Println("Deleted", path)
}

// renterfilesdownloadcmd is the handler for the comand `siac renter download [path] [destination]`.
// Downloads a path from the Sia network to the local specified destination.
func renterfilesdownloadcmd(path, destination string) {
	destination = abs(destination)
	done := make(chan struct{})
	go func() {
		time.Sleep(time.Second) // give download time to initialize
		for {
			select {
			case <-done:
				return

			case <-time.Tick(time.Second):
				// get download progress of file
				var queue api.RenterDownloadQueue
				err := getAPI("/renter/downloads", &queue)
				if err != nil {
					continue // benign
				}
				var d modules.DownloadInfo
				for _, d = range queue.Downloads {
					if d.Destination == destination {
						break
					}
				}
				if d.Filesize == 0 {
					continue // file hasn't appeared in queue yet
				}
				pct := 100 * float64(d.Received) / float64(d.Filesize)
				elapsed := time.Since(d.StartTime)
				elapsed -= elapsed % time.Second // round to nearest second
				mbps := (float64(d.Received*8) / 1e6) / time.Since(d.StartTime).Seconds()
				fmt.Printf("\rDownloading... %5.1f%% of %v, %v elapsed, %.2f Mbps    ", pct, filesizeUnits(int64(d.Filesize)), elapsed, mbps)
			}
		}
	}()

	err := get("/renter/download/" + path + "?destination=" + destination)
	close(done)
	if err != nil {
		die("Could not download file:", err)
	}
	fmt.Printf("\nDownloaded '%s' to %s.\n", path, abs(destination))
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
	err := getAPI("/renter/files", &rf)
	if err != nil {
		die("Could not get file list:", err)
	}
	if len(rf.Files) == 0 {
		fmt.Println("No files have been uploaded.")
		return
	}
	fmt.Println("Tracking", len(rf.Files), "files:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if renterListVerbose {
		fmt.Fprintln(w, "File size\tAvailable\tProgress\tRedundancy\tRenewing\tSia path")
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
			if file.UploadProgress == -1 {
				uploadProgressStr = "-"
			}
			fmt.Fprintf(w, "\t%s\t%8s\t%10s\t%s", availableStr, uploadProgressStr, redundancyStr, renewingStr)
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
	err := post("/renter/rename/"+path, "newsiapath="+newpath)
	if err != nil {
		die("Could not rename file:", err)
	}
	fmt.Printf("Renamed %s to %s\n", path, newpath)
}

// renterfilesuploadcmd is the handler for the command `siac renter upload [source] [path]`.
// Uploads the [source] file to [path] on the Sia network.
func renterfilesuploadcmd(source, path string) {
	err := post("/renter/upload/"+path, "source="+abs(source))
	if err != nil {
		die("Could not upload file:", err)
	}
	fmt.Printf("Uploaded '%s' as %s.\n", abs(source), path)
}

// renterpricescmd is the handler for the command `siac renter prices`, which
// displays the pirces of various storage operations.
func renterpricescmd() {
	var rpg api.RenterPricesGET
	err := getAPI("/renter/prices", &rpg)
	if err != nil {
		die("Could not read the renter prices:", err)
	}

	fmt.Println("Renter Prices (estimated):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tFees for Creating a Set of Contracts:\t", rpg.FormContracts.Div(types.SiacoinPrecision))
	fmt.Fprintln(w, "\tDownload 1 TB:\t", rpg.DownloadTerabyte.Div(types.SiacoinPrecision))
	fmt.Fprintln(w, "\tStore 1 TB for 1 Month:\t", rpg.StorageTerabyteMonth.Div(types.SiacoinPrecision))
	fmt.Fprintln(w, "\tUpload 1 TB:\t", rpg.UploadTerabyte.Div(types.SiacoinPrecision))
	w.Flush()
}
