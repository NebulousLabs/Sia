package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

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
		Use:   "delete",
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
		Use:   "share [nickname] [filepath]",
		Short: "Export a file to a .sia for sharing",
		Long:  "Export a file to a .sia for sharing.",
		Run:   wrap(renterfilessharecmd),
	}

	renterFilesShareASCIICmd = &cobra.Command{
		Use:   "share [nickname]",
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
	var queue []api.DownloadInfo
	err := getAPI("/renter/downloadqueue", &queue)
	if err != nil {
		fmt.Println("Could not get download queue:", err)
		return
	}
	if len(queue) == 0 {
		fmt.Println("No downloads to show.")
		return
	}
	fmt.Println("Download Queue:")
	for _, file := range queue {
		fmt.Printf("%5.1f%% %s -> %s\n", 100*float32(file.Received)/float32(file.Filesize), file.Nickname, file.Destination)
	}
}

func renterfilesdeletecmd(nickname string) {
	err := callAPI(fmt.Sprintf("/renter/files/delete?nickname=%s", nickname))
	if err != nil {
		fmt.Println("Could not delete file:", err)
		return
	}
	fmt.Println("Deleted", nickname)
}

func renterfilesdownloadcmd(nickname, destination string) {
	err := callAPI(fmt.Sprintf("/renter/files/download?nickname=%s&destination=%s", nickname, abs(destination)))
	if err != nil {
		fmt.Println("Could not download file:", err)
		return
	}
	fmt.Printf("Started downloading '%s' to %s.\n", nickname, abs(destination))
}

func renterfileslistcmd() {
	var files []api.FileInfo
	err := getAPI("/renter/files/list", &files)
	if err != nil {
		fmt.Println("Could not get file list:", err)
		return
	}
	if len(files) == 0 {
		fmt.Println("No files have been uploaded.")
		return
	}
	fmt.Println("Tracking", len(files), "files:")
	for _, file := range files {
		fmt.Println("\t", file.Nickname)
	}
}

func renterfilesloadcmd(filename string) {
	info := new(api.RenterFilesLoadResponse)
	err := getAPI(fmt.Sprintf("/renter/files/load?filename=%s", filename), info)
	if err != nil {
		fmt.Println("Could not load file:", err)
		return
	}
	fmt.Printf("Loaded %d files:\n", len(info.FilesAdded))
	for _, file := range info.FilesAdded {
		fmt.Printf("\t%s\n", file)
	}
}

func renterfilesloadasciicmd(data string) {
	info := new(api.RenterFilesLoadResponse)
	err := getAPI(fmt.Sprintf("/renter/files/loadascii?file=%s", data), info)
	if err != nil {
		fmt.Println("Could not load file:", err)
		return
	}
	fmt.Printf("Loaded %d files:\n", len(info.FilesAdded))
	for _, file := range info.FilesAdded {
		fmt.Printf("\t%s\n", file)
	}
}

func renterfilesrenamecmd(nickname, newname string) {
	err := callAPI(fmt.Sprintf("/renter/files/rename?nickname=%s&newname=%s", nickname, newname))
	if err != nil {
		fmt.Println("Could not rename file:", err)
		return
	}
	fmt.Printf("Renamed %s to %s\n", nickname, newname)
}

func renterfilessharecmd(nickname, destination string) {
	err := callAPI(fmt.Sprintf("/renter/files/share?nickname=%s&filepath=%s", nickname, abs(destination)))
	if err != nil {
		fmt.Println("Could not share file:", err)
		return
	}
	fmt.Printf("Exported %s to %s\n", nickname, abs(destination))
}

func renterfilesshareasciicmd(nickname string) {
	var data string
	err := getAPI(fmt.Sprintf("/renter/files/shareascii?nickname=%s", nickname), &data)
	if err != nil {
		fmt.Println("Could not share file:", err)
		return
	}
	fmt.Println(data)
}

func renterfilesuploadcmd(source, nickname string) {
	err := callAPI(fmt.Sprintf("/renter/files/upload?source=%s&nickname=%s", source, nickname))
	if err != nil {
		fmt.Println("Could not upload file:", err)
		return
	}
	fmt.Println("Upload initiated.")
}
