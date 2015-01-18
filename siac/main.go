package main

import (
	"os"
	"reflect"

	"github.com/spf13/cobra"
)

const (
	VERSION  = "0.2.0"
	hostname = "http://localhost:9980"
)

// wrap wraps a generic command with a check that the command has been
// passed the correct number of arguments. The command must take only strings
// as arguments.
func wrap(fn interface{}) func(*cobra.Command, []string) {
	fnVal, fnType := reflect.ValueOf(fn), reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		panic("wrapped function has wrong type signature")
	}
	for i := 0; i < fnType.NumIn(); i++ {
		if fnType.In(i).Kind() != reflect.String {
			panic("wrapped function has wrong type signature")
		}
	}

	return func(cmd *cobra.Command, args []string) {
		if len(args) != fnType.NumIn() {
			cmd.Usage()
			return
		}
		argVals := make([]reflect.Value, fnType.NumIn())
		for i := range args {
			argVals[i] = reflect.ValueOf(args[i])
		}
		fnVal.Call(argVals)
	}
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Client v" + VERSION,
		Long:  "Sia Client v" + VERSION,
		Run:   wrap(func() { println("Sia Client v" + VERSION) }),
	}

	// create command tree
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information.",
		Run:   wrap(func() { println("Sia Client v" + VERSION) }),
	})

	root.AddCommand(hostCmd)
	hostCmd.AddCommand(hostConfigCmd)
	hostCmd.AddCommand(hostSetConfigCmd)

	root.AddCommand(minerCmd)
	minerCmd.AddCommand(minerStartCmd)
	minerCmd.AddCommand(minerStatusCmd)
	minerCmd.AddCommand(minerStopCmd)

	root.AddCommand(walletCmd)
	walletCmd.AddCommand(walletAddressCmd)
	walletCmd.AddCommand(walletSendCmd)
	walletCmd.AddCommand(walletStatusCmd)

	root.AddCommand(fileCmd)
	fileCmd.AddCommand(fileUploadCmd)
	fileCmd.AddCommand(fileDownloadCmd)
	fileCmd.AddCommand(fileStatusCmd)

	root.AddCommand(peerCmd)
	peerCmd.AddCommand(peerAddCmd)
	peerCmd.AddCommand(peerRemoveCmd)
	peerCmd.AddCommand(peerStatusCmd)

	root.AddCommand(updateCmd)
	updateCmd.AddCommand(updateCheckCmd)
	updateCmd.AddCommand(updateApplyCmd)
	root.AddCommand(statusCmd)
	root.AddCommand(stopCmd)
	root.AddCommand(syncCmd)

	// run
	root.Execute()
}
