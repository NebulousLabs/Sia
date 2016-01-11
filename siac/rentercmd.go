package main

import (
	"fmt"
	"math"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

// filesize returns a string that displays a filesize in human-readable units.
func filesizeUnits(size int64) string {
	if size == 0 {
		return "0 B"
	}
	sizes := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}
	i := int(math.Log10(float64(size)) / 3)
	return fmt.Sprintf("%.*f %s", i, float64(size)/math.Pow10(3*i), sizes[i])
}

var (
	renterCmd = &cobra.Command{
		Use:   "renter",
		Short: "Perform renter actions",
		Long:  "Upload, download, rename, delete, load, or share files.",
		Run:   wrap(renterfileslistcmd),
	}

	renterDownloadQueueCmd = &cobra.Command{
		Use:   "queue",
		Short: "View the download queue",
		Long:  "View the list of files that have been downloaded.",
		Run:   wrap(renterdownloadqueuecmd),
	}

	renterFilesDeleteCmd = &cobra.Command{
		Use:   "delete [path]",
		Short: "Delete a file",
		Long:  "Delete a file. Does not delete the file on disk.",
		Run:   wrap(renterfilesdeletecmd),
	}

	renterFilesDownloadCmd = &cobra.Command{
		Use:   "download [path] [destination]",
		Short: "Download a file",
		Long:  "Download a previously-uploaded file to a specified destination.",
		Run:   wrap(renterfilesdownloadcmd),
	}

	renterFilesListCmd = &cobra.Command{
		Use:   "list",
		Short: "List the status of all files",
		Long:  "List the status of all files known to the renter.",
		Run:   wrap(renterfileslistcmd),
	}

	renterFilesLoadCmd = &cobra.Command{
		Use:   "load [source]",
		Short: "Load a .sia file",
		Long:  "Load a .sia file, adding the file entries contained within.",
		Run:   wrap(renterfilesloadcmd),
	}

	renterFilesLoadASCIICmd = &cobra.Command{
		Use:   "loadascii [ascii]",
		Short: "Load an ASCII-encoded .sia file",
		Long:  "Load an ASCII-encoded .sia file.",
		Run:   wrap(renterfilesloadasciicmd),
	}

	renterFilesRenameCmd = &cobra.Command{
		Use:   "rename [path] [newpath]",
		Short: "Rename a file",
		Long:  "Rename a file.",
		Run:   wrap(renterfilesrenamecmd),
	}

	renterFilesShareCmd = &cobra.Command{
		Use:   "share [path] [destination]",
		Short: "Export a file to a .sia for sharing",
		Long:  "Export a file to a .sia for sharing.",
		Run:   wrap(renterfilessharecmd),
	}

	renterFilesShareASCIICmd = &cobra.Command{
		Use:   "shareascii [path]",
		Short: "Export a file as an ASCII-encoded .sia file",
		Long:  "Export a file as an ASCII-encoded .sia file.",
		Run:   wrap(renterfilesshareasciicmd),
	}

	renterFilesUploadCmd = &cobra.Command{
		Use:   "upload [source] [path]",
		Short: "Upload a file",
		Long:  "Upload a file using a given nickname.",
		Run:   wrap(renterfilesuploadcmd),
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

func renterdownloadqueuecmd() {
	var queue api.RenterDownloadQueue
	err := getAPI("/renter/downloads", &queue)
	if err != nil {
		fmt.Println("Could not get download queue:", err)
		return
	}
	if len(queue.Downloads) == 0 {
		fmt.Println("No downloads to show.")
		return
	}
	fmt.Println("Download Queue:")
	for _, file := range queue.Downloads {
		fmt.Printf("%s: %5.1f%% %s -> %s\n", file.StartTime.Format("Jan 02 03:04 PM"), 100*float64(file.Received)/float64(file.Filesize), file.SiaPath, file.Destination)
	}
}

func renterfilesdeletecmd(path string) {
	err := post("/renter/delete/"+path, "")
	if err != nil {
		fmt.Println("Could not delete file:", err)
		return
	}
	fmt.Println("Deleted", path)
}

func renterfilesdownloadcmd(path, destination string) {
	err := get("/renter/download/" + path + "?destination=" + abs(destination))
	if err != nil {
		fmt.Println("Could not download file:", err)
		return
	}
	fmt.Printf("Downloaded '%s' to %s.\n", path, abs(destination))
}

func renterfileslistcmd() {
	var rf api.RenterFiles
	err := getAPI("/renter/files", &rf)
	if err != nil {
		fmt.Println("Could not get file list:", err)
		return
	}
	if len(rf.Files) == 0 {
		fmt.Println("No files have been uploaded.")
		return
	}
	fmt.Println("Tracking", len(rf.Files), "files:")
	for _, file := range rf.Files {
		if file.Available {
			fmt.Printf("%13s  %s\n", filesizeUnits(int64(file.Filesize)), file.SiaPath)
		} else {
			fmt.Printf("%13s  %s (uploading, %0.2f%%)\n", filesizeUnits(int64(file.Filesize)), file.SiaPath, file.UploadProgress)
		}
	}
}

func renterfilesloadcmd(source string) {
	var info api.RenterLoad
	err := postResp("/renter/load", "source="+abs(source), &info)
	if err != nil {
		fmt.Println("Could not load file:", err)
		return
	}
	fmt.Printf("Loaded %d file(s):\n", len(info.FilesAdded))
	for _, file := range info.FilesAdded {
		fmt.Printf("\t%s\n", file)
	}
}

func renterfilesloadasciicmd(ascii string) {
	var info api.RenterLoad
	err := postResp("/renter/loadascii", "asciisia="+ascii, &info)
	if err != nil {
		fmt.Println("Could not load file:", err)
		return
	}
	fmt.Printf("Loaded %d file(s):\n", len(info.FilesAdded))
	for _, file := range info.FilesAdded {
		fmt.Printf("\t%s\n", file)
	}
}

func renterfilesrenamecmd(path, newpath string) {
	err := post("/renter/rename/"+path, "newsiapath="+newpath)
	if err != nil {
		fmt.Println("Could not rename file:", err)
		return
	}
	fmt.Printf("Renamed %s to %s\n", path, newpath)
}

func renterfilessharecmd(path, destination string) {
	err := get(fmt.Sprintf("/renter/share?siapaths=%s&destination=%s", path, abs(destination)))
	if err != nil {
		fmt.Println("Could not share file:", err)
		return
	}
	fmt.Printf("Exported %s to %s\n", path, abs(destination))
}

func renterfilesshareasciicmd(path string) {
	var data api.RenterShareASCII
	err := getAPI("/renter/shareascii?siapaths="+path, &data)
	if err != nil {
		fmt.Println("Could not share file:", err)
		return
	}
	fmt.Println(data.ASCIIsia)
}

func renterfilesuploadcmd(source, path string) {
	err := post("/renter/upload/"+path, "source="+abs(source))
	if err != nil {
		fmt.Println("Could not upload file:", err)
		return
	}
	fmt.Printf("Uploaded '%s' as %s.\n", abs(source), path)
}
