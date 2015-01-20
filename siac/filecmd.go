package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/sia/components"
)

var (
	fileCmd = &cobra.Command{
		Use:   "file",
		Short: "Perform file actions",
		Long:  "Generate a new address, send coins to another file, or view info about the file.",
		Run:   wrap(filestatuscmd),
	}

	fileUploadCmd = &cobra.Command{
		Use:   "upload [filename] [nickname] [pieces]",
		Short: "Upload a file",
		Long:  "Upload a file using a given nickname and split across 'pieces' hosts.",
		Run:   wrap(fileuploadcmd),
	}

	fileDownloadCmd = &cobra.Command{
		Use:   "download [nickname] [filename]",
		Short: "Download a file",
		Long:  "Download a previously-uploaded file to a specified destination.",
		Run:   wrap(filedownloadcmd),
	}

	fileStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View a list of uploaded files",
		Long:  "View a list of files that have been uploaded to the network.",
		Run:   wrap(filestatuscmd),
	}
)

// siac does not support /file/upload, only /file/uploadpath
func fileuploadcmd(filename, nickname, pieces string) {
	err := callAPI(fmt.Sprintf("/file/uploadpath?filename=%s&nickname=%s&pieces=%s", filename, nickname, pieces))
	if err != nil {
		fmt.Println("Could not upload file:", err)
		return
	}
	fmt.Println("Uploaded", filename, "as", nickname)
}

func filedownloadcmd(nickname, filename string) {
	err := callAPI(fmt.Sprintf("/file/download?nickname=%s&filename=%s", nickname, filename))
	if err != nil {
		fmt.Println("Could not download file:", err)
		return
	}
	fmt.Println("Downloaded", nickname, "to", filename)
}

func filestatuscmd() {
	status := new(components.RentInfo)
	err := getAPI("/file/status", status)
	if err != nil {
		fmt.Println("Could not get file status:", err)
		return
	}
	if len(status.Files) == 0 {
		fmt.Println("Not files have been uploaded.")
		return
	}
	fmt.Println("Uploaded", len(status.Files), "files:")
	for _, file := range status.Files {
		fmt.Println("\t", file)
	}
}
