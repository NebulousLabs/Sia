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
		Use:   "delete [nickname]",
		Short: "Delete a file",
		Long:  "Delete a file. Does not delete the file on disk.",
		Run:   wrap(renterfilesdeletecmd),
	}

	renterFilesDownloadCmd = &cobra.Command{
		Use:   "download [nickname] [destination]",
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
		Use:   "load [filename]",
		Short: "Load a .sia file",
		Long:  "Load a .sia file, adding the file entries contained within.",
		Run:   wrap(renterfilesloadcmd),
	}

	renterFilesLoadASCIICmd = &cobra.Command{
		Use:   "loadascii [data]",
		Short: "Load an ASCII-encoded .sia file",
		Long:  "Load an ASCII-encoded .sia file.",
		Run:   wrap(renterfilesloadasciicmd),
	}

	renterFilesRenameCmd = &cobra.Command{
		Use:   "rename [nickname] [newname]",
		Short: "Rename a file",
		Long:  "Rename a file.",
		Run:   wrap(renterfilesrenamecmd),
	}

	renterFilesShareCmd = &cobra.Command{
		Use:   "share [nickname] [destination]",
		Short: "Export a file to a .sia for sharing",
		Long:  "Export a file to a .sia for sharing.",
		Run:   wrap(renterfilessharecmd),
	}

	renterFilesShareASCIICmd = &cobra.Command{
		Use:   "shareascii [nickname]",
		Short: "Export a file as an ASCII-encoded .sia file",
		Long:  "Export a file as an ASCII-encoded .sia file.",
		Run:   wrap(renterfilesshareasciicmd),
	}

	renterFilesUploadCmd = &cobra.Command{
		Use:   "upload [filename] [nickname]",
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
	err := getAPI("/renter/downloadqueue", &queue)
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
		fmt.Printf("%s: %5.1f%% %s -> %s\n", file.StartTime.Format("Jan 02 03:04 PM"), 100*float32(file.Received)/float32(file.Filesize), file.Nickname, file.Destination)
	}
}

func renterfilesdeletecmd(nickname string) {
	err := post("/renter/delete/"+nickname, "")
	if err != nil {
		fmt.Println("Could not delete file:", err)
		return
	}
	fmt.Println("Deleted", nickname)
}

func renterfilesdownloadcmd(nickname, destination string) {
	err := post("/renter/download/"+nickname, "destination="+abs(destination))
	if err != nil {
		fmt.Println("Could not download file:", err)
		return
	}
	fmt.Printf("Downloaded '%s' to %s.\n", nickname, abs(destination))
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
		// TODO: write a filesize() helper function to display proper units
		if file.Available {
			fmt.Printf("%13s  %s\n", filesizeUnits(int64(file.Filesize)), file.Nickname)
		} else {
			fmt.Printf("%13s  %s (uploading, %0.2f%%)\n", filesizeUnits(int64(file.Filesize)), file.Nickname, file.UploadProgress)
		}
	}
}

func renterfilesloadcmd(filename string) {
	var info api.RenterLoad
	err := postResp("/renter/load", "filename="+abs(filename), &info)
	if err != nil {
		fmt.Println("Could not load file:", err)
		return
	}
	fmt.Printf("Loaded %d file(s):\n", len(info.FilesAdded))
	for _, file := range info.FilesAdded {
		fmt.Printf("\t%s\n", file)
	}
}

func renterfilesloadasciicmd(data string) {
	var info api.RenterLoad
	err := postResp("/renter/loadascii", "file="+data, &info)
	if err != nil {
		fmt.Println("Could not load file:", err)
		return
	}
	fmt.Printf("Loaded %d file(s):\n", len(info.FilesAdded))
	for _, file := range info.FilesAdded {
		fmt.Printf("\t%s\n", file)
	}
}

func renterfilesrenamecmd(nickname, newname string) {
	err := post("/renter/rename", fmt.Sprintf("nickname=%s&newname=%s", nickname, newname))
	if err != nil {
		fmt.Println("Could not rename file:", err)
		return
	}
	fmt.Printf("Renamed %s to %s\n", nickname, newname)
}

func renterfilessharecmd(nickname, destination string) {
	err := get(fmt.Sprintf("/renter/share/%s?destination=%s", nickname, abs(destination)))
	if err != nil {
		fmt.Println("Could not share file:", err)
		return
	}
	fmt.Printf("Exported %s to %s\n", nickname, abs(destination))
}

func renterfilesshareasciicmd(nickname string) {
	var data api.RenterShareASCII
	err := getAPI("/renter/shareascii/"+nickname, &data)
	if err != nil {
		fmt.Println("Could not share file:", err)
		return
	}
	fmt.Println(data.File)
}

func renterfilesuploadcmd(source, nickname string) {
	err := post("/renter/upload/"+nickname, "source="+abs(source))
	if err != nil {
		fmt.Println("Could not upload file:", err)
		return
	}
	fmt.Printf("Uploaded '%s' as %s.\n", abs(source), nickname)
}
