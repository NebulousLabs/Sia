package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/build"
)

var (
	addr         string
	force        bool
	initPassword bool
)

// apiGet wraps a GET request with a status code check, such that if the GET does
// not return 200, the error will be read and returned. The response body is
// not closed.
func apiGet(call string) (*http.Response, error) {
	if host, port, _ := net.SplitHostPort(addr); host == "" {
		addr = net.JoinHostPort("localhost", port)
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://"+addr+call, nil)
	if err != nil {
		return nil, errors.New("no response from daemon")
	}
	req.Header.Add("User-Agent", "Sia-Agent")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("no response from daemon")
	}
	// check error code
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

	client := &http.Client{}
	req, err := http.NewRequest("POST", "http://"+addr+call, strings.NewReader(vals))
	if err != nil {
		return nil, errors.New("no response from daemon")
	}
	req.Header.Add("User-Agent", "Sia-Agent")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("no response from daemon")
	}
	// check error code
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
			return
		}
		argVals := make([]reflect.Value, fnType.NumIn())
		for i := range args {
			argVals[i] = reflect.ValueOf(args[i])
		}
		fnVal.Call(argVals)
	}
}

func version(*cobra.Command, []string) {
	println("Sia Client v" + build.Version)
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Client v" + build.Version,
		Long:  "Sia Client v" + build.Version,
		Run:   version,
	}

	// create command tree
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information.",
		Run:   version,
	})

	root.AddCommand(hostCmd)
	hostCmd.AddCommand(hostConfigCmd, hostAnnounceCmd, hostStatusCmd)

	root.AddCommand(hostdbCmd)
	hostCmd.AddCommand(hostdbCmd)

	root.AddCommand(minerCmd)
	minerCmd.AddCommand(minerStartCmd, minerStopCmd, minerStatusCmd)

	root.AddCommand(walletCmd)
	walletCmd.AddCommand(walletAddressCmd, walletAddressesCmd, walletAddseedCmd, walletInitCmd, walletLoad033xCmd, walletLockCmd, walletSeedsCmd, walletSendCmd, walletStatusCmd, walletUnlockCmd)
	walletInitCmd.Flags().BoolVarP(&initPassword, "password", "p", false, "Prompt for a custom password")

	root.AddCommand(renterCmd)
	renterCmd.AddCommand(renterDownloadQueueCmd, renterFilesDeleteCmd, renterFilesDownloadCmd,
		renterFilesListCmd, renterFilesLoadCmd, renterFilesLoadASCIICmd, renterFilesRenameCmd,
		renterFilesShareCmd, renterFilesShareASCIICmd, renterFilesUploadCmd)

	root.AddCommand(gatewayCmd)
	gatewayCmd.AddCommand(gatewayAddCmd, gatewayRemoveCmd, gatewayStatusCmd)

	root.AddCommand(updateCmd)
	updateCmd.AddCommand(updateCheckCmd, updateApplyCmd)

	// consensus cmds have no leading qualifier
	root.AddCommand(consensusStatusCmd)
	root.AddCommand(stopCmd)

	// parse flags
	root.PersistentFlags().StringVarP(&addr, "addr", "a", "localhost:9980", "which host/port to communicate with (i.e. the host/port siad is listening on)")
	root.PersistentFlags().BoolVarP(&force, "force", "f", false, "force certain commands")

	// run
	root.Execute()
}
