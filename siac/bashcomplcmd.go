package main

import "github.com/spf13/cobra"

var (
	bashcomplCmd = &cobra.Command{
		Use:   "bash-completion [path]",
		Short: "Creates bash completion file.",
		Long: "Creates a bash completion file at the specified " +
			"location.\n\n" +

			"Note: Bash completions will only work with the " +
			"prefix with which the script is created (e.g. " +
			"`./siac` or `siac`).\n\n" +

			"Once created, the file has to be moved to the bash " +
			"completion script folder - usually " +
			"`/etc/bash_completion.d/`.",
		Run: wrap(bashcomplcmd),
	}
)

func bashcomplcmd(path string) {
	rootCmd.GenBashCompletionFile(path)
}
