package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	fileCmd = &cobra.Command{
		Use:   "file [upload|download|status]",
		Short: "Perform file actions",
		Long:  "Generate a new address, send coins to another file, or view info about the file.",
		Run:   wrap(filestatuscmd),
	}

	fileUploadCmd = &cobra.Command{
		Use:   "file upload [file] [nickname] [pieces]",
		Short: "Upload a file",
		Long:  "Upload a file using a given nickname and split across 'pieces' hosts.",
		Run:   wrap(fileuploadcmd),
	}

	fileDownloadCmd = &cobra.Command{
		Use:   "file download [nickname] [filename]",
		Short: "Download a file",
		Long:  "Download a previously-uploaded file to a specified destination.",
		Run:   wrap(filedownloadcmd),
	}

	fileStatusCmd = &cobra.Command{
		Use:   "file status",
		Short: "View a list of uploaded files",
		Long:  "View a list of files that have been uploaded to the network.",
		Run:   wrap(filestatuscmd),
	}
)

func fileuploadcmd(file, nickname, pieces string) {
	err := getFileUpload(file, nickname, pieces)
	if err != nil {
		fmt.Println("Could not upload file:", err)
		return
	}
	fmt.Println("Uploaded", file, "as", nickname)
}

func filedownloadcmd(nickname, filename string) {
	err := getFileDownload(nickname, filename)
	if err != nil {
		fmt.Println("Could not download file:", err)
		return
	}
	fmt.Println("Downloaded", nickname, "to", filename)
}

func filestatuscmd() {
	status, err := getFileStatus()
	if err != nil {
		fmt.Println("Could not get file status:", err)
		return
	}
	fmt.Println(status)
}
