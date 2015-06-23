package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/build"
)

var (
	port string
)

// apiGet wraps a GET request with a status code check, such that if the GET does
// not return 200, the error will be read and returned. The response body is
// not closed.
func apiGet(call string) (*http.Response, error) {
	resp, err := http.Get("http://localhost:" + port + call)
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
	data, err := url.ParseQuery(vals)
	if err != nil {
		return nil, errors.New("bad query string")
	}
	resp, err := http.PostForm("http://localhost:"+port+call, data)
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
	walletCmd.AddCommand(walletAddressCmd, walletMergeCmd, walletSendCmd, walletSiafundsCmd, walletStatusCmd)
	walletSiafundsCmd.AddCommand(walletSiafundsTrackCmd)
	walletSiafundsCmd.AddCommand(walletSiafundsSendCmd)

	root.AddCommand(renterCmd)
	renterCmd.AddCommand(renterDownloadQueueCmd, renterFilesDeleteCmd, renterFilesDownloadCmd,
		renterFilesListCmd, renterFilesLoadCmd, renterFilesLoadASCIICmd, renterFilesRenameCmd,
		renterFilesShareCmd, renterFilesShareASCIICmd, renterFilesUploadCmd)

	root.AddCommand(gatewayCmd)
	gatewayCmd.AddCommand(gatewayAddCmd, gatewayRemoveCmd, gatewayStatusCmd)

	root.AddCommand(updateCmd)
	updateCmd.AddCommand(updateCheckCmd, updateApplyCmd)

	// consensus cmds have no leading qualifier
	root.AddCommand(consensusSynchronizeCmd, consensusStatusCmd)
	root.AddCommand(stopCmd)

	// parse flags
	root.PersistentFlags().StringVarP(&port, "port", "p", "9980", "which port to communicate with (i.e. the port siad is listening on)")

	// run
	root.Execute()
}
