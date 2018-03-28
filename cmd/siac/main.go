package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/node/api"
)

var (
	// Flags.
	addr              string // override default API address
	hostVerbose       bool   // display additional host info
	initForce         bool   // destroy and reencrypt the wallet on init if it already exists
	initPassword      bool   // supply a custom password when creating a wallet
	renterListVerbose bool   // Show additional info about uploaded files.
	renterShowHistory bool   // Show download history in addition to download queue.
)

var (
	// Globals.
	rootCmd *cobra.Command // Root command cobra object, used by bash completion cmd.
)

var (
	// User-supplied password, cached so that we don't need to prompt multiple
	// times.
	apiPassword string
)

// Exit codes.
// inspired by sysexits.h
const (
	exitCodeGeneral = 1  // Not in sysexits.h, but is standard practice.
	exitCodeUsage   = 64 // EX_USAGE in sysexits.h
)

// non2xx returns true for non-success HTTP status codes.
func non2xx(code int) bool {
	return code < 200 || code > 299
}

// decodeError returns the api.Error from a API response. This method should
// only be called if the response's status code is non-2xx. The error returned
// may not be of type api.Error in the event of an error unmarshalling the
// JSON.
func decodeError(resp *http.Response) error {
	var apiErr api.Error
	err := json.NewDecoder(resp.Body).Decode(&apiErr)
	if err != nil {
		return err
	}
	return apiErr
}

// apiGet wraps a GET request with a status code check, such that if the GET does
// not return 2xx, the error will be read and returned. The response body is
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
		// retry request with authentication.
		resp.Body.Close()
		if apiPassword == "" {
			apiPassword = os.Getenv("SIA_API_PASSWORD")
			if apiPassword != "" {
				fmt.Println("Using SIA_API_PASSWORD environment variable")
			} else {
				// prompt for password and store it in a global var for subsequent
				// calls
				apiPassword, err = passwordPrompt("API password: ")
				if err != nil {
					return nil, err
				}
			}
		}
		resp, err = api.HttpGETAuthenticated("http://"+addr+call, apiPassword)
		if err != nil {
			return nil, errors.New("no response from daemon - authentication failed")
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, errors.New("API call not recognized: " + call)
	}
	if non2xx(resp.StatusCode) {
		err := decodeError(resp)
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

// getAPI makes a GET API call and decodes the response. An error is returned
// if the response status is not 2xx.
func getAPI(call string, obj interface{}) error {
	resp, err := apiGet(call)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return errors.New("expecting a response, but API returned status code 204 No Content")
	}

	err = json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		return err
	}
	return nil
}

// get makes an API call and discards the response. An error is returned if the
// response status is not 2xx.
func get(call string) error {
	resp, err := apiGet(call)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// apiPost wraps a POST request with a status code check, such that if the POST
// does not return 2xx, the error will be read and returned. The response body
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
		apiPassword = os.Getenv("SIA_API_PASSWORD")
		if apiPassword != "" {
			fmt.Println("Using SIA_API_PASSWORD environment variable")
		} else {
			// Prompt for password and retry request with authentication.
			apiPassword, err = passwordPrompt("API password: ")
			if err != nil {
				return nil, err
			}
		}
		resp, err = api.HttpPOSTAuthenticated("http://"+addr+call, vals, apiPassword)
		if err != nil {
			return nil, errors.New("no response from daemon - authentication failed")
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, errors.New("API call not recognized: " + call)
	}
	if non2xx(resp.StatusCode) {
		err := decodeError(resp)
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

// postResp makes a POST API call and decodes the response. An error is
// returned if the response status is not 2xx.
func postResp(call, vals string, obj interface{}) error {
	resp, err := apiPost(call, vals)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return errors.New("expecting a response, but API returned status code 204 No Content")
	}

	err = json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		return err
	}
	return nil
}

// post makes an API call and discards the response. An error is returned if
// the response status is not 2xx.
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
			cmd.UsageFunc()(cmd)
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

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Client v" + build.Version,
		Long:  "Sia Client v" + build.Version,
		Run:   wrap(consensuscmd),
	}

	rootCmd = root

	// create command tree
	root.AddCommand(versionCmd)
	root.AddCommand(stopCmd)

	root.AddCommand(updateCmd)
	updateCmd.AddCommand(updateCheckCmd)

	root.AddCommand(hostCmd)
	hostCmd.AddCommand(hostConfigCmd, hostAnnounceCmd, hostFolderCmd, hostSectorCmd)
	hostFolderCmd.AddCommand(hostFolderAddCmd, hostFolderRemoveCmd, hostFolderResizeCmd)
	hostSectorCmd.AddCommand(hostSectorDeleteCmd)
	hostCmd.Flags().BoolVarP(&hostVerbose, "verbose", "v", false, "Display detailed host info")

	root.AddCommand(hostdbCmd)
	hostdbCmd.AddCommand(hostdbViewCmd)
	hostdbCmd.Flags().IntVarP(&hostdbNumHosts, "numhosts", "n", 0, "Number of hosts to display from the hostdb")
	hostdbCmd.Flags().BoolVarP(&hostdbVerbose, "verbose", "v", false, "Display full hostdb information")

	root.AddCommand(minerCmd)
	minerCmd.AddCommand(minerStartCmd, minerStopCmd)

	root.AddCommand(walletCmd)
	walletCmd.AddCommand(walletAddressCmd, walletAddressesCmd, walletChangepasswordCmd, walletInitCmd, walletInitSeedCmd,
		walletLoadCmd, walletLockCmd, walletSeedsCmd, walletSendCmd, walletSweepCmd, walletSignCmd,
		walletBalanceCmd, walletTransactionsCmd, walletUnlockCmd)
	walletInitCmd.Flags().BoolVarP(&initPassword, "password", "p", false, "Prompt for a custom password")
	walletInitCmd.Flags().BoolVarP(&initForce, "force", "", false, "destroy the existing wallet and re-encrypt")
	walletInitSeedCmd.Flags().BoolVarP(&initForce, "force", "", false, "destroy the existing wallet")
	walletLoadCmd.AddCommand(walletLoad033xCmd, walletLoadSeedCmd, walletLoadSiagCmd)
	walletSendCmd.AddCommand(walletSendSiacoinsCmd, walletSendSiafundsCmd)
	walletUnlockCmd.Flags().BoolVarP(&initPassword, "password", "p", false, "Display interactive password prompt even if SIA_WALLET_PASSWORD is set")

	root.AddCommand(renterCmd)
	renterCmd.AddCommand(renterFilesDeleteCmd, renterFilesDownloadCmd,
		renterDownloadsCmd, renterAllowanceCmd, renterSetAllowanceCmd,
		renterContractsCmd, renterFilesListCmd, renterFilesRenameCmd,
		renterFilesUploadCmd, renterUploadsCmd, renterExportCmd,
		renterPricesCmd)

	renterContractsCmd.AddCommand(renterContractsViewCmd)
	renterAllowanceCmd.AddCommand(renterAllowanceCancelCmd)

	renterCmd.Flags().BoolVarP(&renterListVerbose, "verbose", "v", false, "Show additional file info such as redundancy")
	renterDownloadsCmd.Flags().BoolVarP(&renterShowHistory, "history", "H", false, "Show download history in addition to the download queue")
	renterFilesListCmd.Flags().BoolVarP(&renterListVerbose, "verbose", "v", false, "Show additional file info such as redundancy")
	renterExportCmd.AddCommand(renterExportContractTxnsCmd)

	root.AddCommand(gatewayCmd)
	gatewayCmd.AddCommand(gatewayConnectCmd, gatewayDisconnectCmd, gatewayAddressCmd, gatewayListCmd)

	root.AddCommand(consensusCmd)

	root.AddCommand(bashcomplCmd)
	root.AddCommand(mangenCmd)

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
