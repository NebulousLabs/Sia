package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var (
	mangenCmd = &cobra.Command{
		Use:   "man-generation [path]",
		Short: "Creates unix style manpages.",
		Long:  "Creates unix style man pages at the specified directory.",
		Run:   wrap(mangencmd),
	}
)

func mangencmd(path string) {
	header := &doc.GenManHeader{
		Section: "1",
		Manual:  "siac Manual",
		Source:  "",
	}

	doc.GenManTree(rootCmd, header, path)
}
