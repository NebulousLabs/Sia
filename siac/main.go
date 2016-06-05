package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/bgentry/speakeasy"
	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/build"
)

// flags
var (
	addr              string // override default API address
	initPassword      bool   // supply a custom password when creating a wallet
	hostVerbose       bool   // display additional host info
	renterShowHistory bool   // Show download history in addition to download queue.
	renterListVerbose bool   // Show additional info about uploaded files.
)

// exit codes
// inspired by sysexits.h
const (
	exitCodeGeneral = 1  // Not in sysexits.h, but is standard practice.
	exitCodeUsage   = 64 // EX_USAGE in sysexits.h
)

// apiGet wraps a GET request with a status code check, such that if the GET does
// not return 200, the error will be read and returned. The response body is
// not closed.
func apiGet(call string) (*http.Response, error) {
	if host, port, _ := net.SplitHostPort(addr); host == "" {
		addr = net.JoinHostPort("localhost", port)
	}
	resp, err := api.HttpGET("http://" + addr + call)
	if err != nil {
		return nil, errors.New("no response from daemon")
	}
	// check error code
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		// Prompt for password and retry request with authentication.
		password, err := speakeasy.Ask("API password: ")
		if err != nil {
			return nil, err
		}
		resp, err = api.HttpGETAuthenticated("http://"+addr+call, password)
		if err != nil {
			return nil, errors.New("no response from daemon - authentication failed")
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		err = errors.New("API call not recognized: " + call)
	} else if resp.StatusCode != http.StatusOK {
		errResp, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		err = errors.New(strings.TrimSpace(string(errResp)))
	}
	return resp, err
}

// getAPI makes a GET API call and decodes the response.
func getAPI(call string, obj interface{}) error {
	resp, err := apiGet(call)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		return err
	}
	return nil
}

// get makes an API call and discards the response.
func get(call string) error {
	resp, err := apiGet(call)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// apiPost wraps a POST request with a status code check, such that if the POST
// does not return 200, the error will be read and returned. The response body
// is not closed.
func apiPost(call, vals string) (*http.Response, error) {
	if host, port, _ := net.SplitHostPort(addr); host == "" {
		addr = net.JoinHostPort("localhost", port)
	}

	resp, err := api.HttpPOST("http://"+addr+call, vals)
	if err != nil {
		return nil, errors.New("no response from daemon")
	}
	// check error code
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		// Prompt for password and retry request with authentication.
		password, err := speakeasy.Ask("API password: ")
		if err != nil {
			return nil, err
		}
		resp, err = api.HttpPOSTAuthenticated("http://"+addr+call, vals, password)
		if err != nil {
			return nil, errors.New("no response from daemon - authentication failed")
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		err = errors.New("API call not recognized: " + call)
	} else if resp.StatusCode != http.StatusOK {
		errResp, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		err = errors.New(strings.TrimSpace(string(errResp)))
	}
	return resp, err
}

// postResp makes a POST API call and decodes the response.
func postResp(call, vals string, obj interface{}) error {
	resp, err := apiPost(call, vals)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		return err
	}
	return nil
}

func post(call, vals string) error {
	resp, err := apiPost(call, vals)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

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
			os.Exit(exitCodeUsage)
		}
		argVals := make([]reflect.Value, fnType.NumIn())
		for i := range args {
			argVals[i] = reflect.ValueOf(args[i])
		}
		fnVal.Call(argVals)
	}
}

// die prints its arguments to stderr, then exits the program with the default
// error code.
func die(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCodeGeneral)
}

func version() {
	println("Sia Client v" + build.Version)
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Client v" + build.Version,
		Long:  "Sia Client v" + build.Version,
		Run:   wrap(consensuscmd),
	}

	// create command tree
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information.",
		Run:   wrap(version),
	})

	root.AddCommand(stopCmd)

	root.AddCommand(hostCmd)
	hostCmd.AddCommand(hostConfigCmd, hostAnnounceCmd, hostFolderCmd, hostSectorCmd)
	hostFolderCmd.AddCommand(hostFolderAddCmd, hostFolderRemoveCmd, hostFolderResizeCmd)
	hostSectorCmd.AddCommand(hostSectorDeleteCmd)
	hostCmd.Flags().BoolVarP(&hostVerbose, "verbose", "v", false, "Display detailed host info")

	root.AddCommand(hostdbCmd)

	root.AddCommand(minerCmd)
	minerCmd.AddCommand(minerStartCmd, minerStopCmd)

	root.AddCommand(walletCmd)
	walletCmd.AddCommand(walletAddressCmd, walletAddressesCmd, walletInitCmd,
		walletLoadCmd, walletLockCmd, walletSeedsCmd, walletSendCmd,
		walletBalanceCmd, walletTransactionsCmd, walletUnlockCmd)
	walletInitCmd.Flags().BoolVarP(&initPassword, "password", "p", false, "Prompt for a custom password")
	walletLoadCmd.AddCommand(walletLoad033xCmd, walletLoadSeedCmd, walletLoadSiagCmd)
	walletSendCmd.AddCommand(walletSendSiacoinsCmd, walletSendSiafundsCmd)

	root.AddCommand(renterCmd)
	renterCmd.AddCommand(renterFilesDeleteCmd, renterFilesDownloadCmd,
		renterDownloadsCmd, renterAllowanceCmd, renterSetAllowanceCmd,
		renterContractsCmd, renterFilesListCmd, renterFilesLoadCmd,
		renterFilesLoadASCIICmd, renterFilesRenameCmd, renterFilesShareCmd,
		renterFilesShareASCIICmd, renterFilesUploadCmd, renterUploadsCmd)
	renterCmd.Flags().BoolVarP(&renterListVerbose, "verbose", "v", false, "Show additional file info such as redundancy")
	renterDownloadsCmd.Flags().BoolVarP(&renterShowHistory, "history", "H", false, "Show download history in addition to the download queue")
	renterFilesListCmd.Flags().BoolVarP(&renterListVerbose, "verbose", "v", false, "Show additional file info such as redundancy")

	root.AddCommand(gatewayCmd)
	gatewayCmd.AddCommand(gatewayConnectCmd, gatewayDisconnectCmd, gatewayAddressCmd, gatewayListCmd)

	root.AddCommand(consensusCmd)

	// COMPATv0.6.0
	gatewayCmd.AddCommand(gatewayDeprecatedAddCmd)
	gatewayCmd.AddCommand(gatewayDeprecatedRemoveCmd)

	// parse flags
	root.PersistentFlags().StringVarP(&addr, "addr", "a", "localhost:9980", "which host/port to communicate with (i.e. the host/port siad is listening on)")

	// run
	if err := root.Execute(); err != nil {
		// Since no commands return errors (all commands set Command.Run instead of
		// Command.RunE), Command.Execute() should only return an error on an
		// invalid command or flag. Therefore Command.Usage() was called (assuming
		// Command.SilenceUsage is false) and we should exit with exitCodeUsage.
		os.Exit(exitCodeUsage)
	}
}
