package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
)

const (
	VERSION = "0.3.0"
)

var (
	port string
)

// get wraps a GET request with a status code check, such that if the GET does
// not return 200, the error will be read and returned. The response body is
// not closed.
func get(call string) (resp *http.Response, err error) {
	resp, err = http.Get("http://localhost:" + port + call)
	if err != nil {
		return nil, errors.New("no response from daemon")
	}
	// check error code
	if resp.StatusCode == http.StatusNotFound {
		err = errors.New("API call not recognized: " + call)
	} else if resp.StatusCode != http.StatusOK {
		errResp, _ := ioutil.ReadAll(resp.Body)
		err = errors.New(strings.TrimSpace(string(errResp)))
	}
	return
}

// getAPI makes an API call and decodes the response.
func getAPI(call string, obj interface{}) (err error) {
	resp, err := get(call)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		return
	}
	return
}

// callAPI makes an API call and discards the response.
func callAPI(call string) (err error) {
	resp, err := get(call)
	if err != nil {
		return
	}
	resp.Body.Close()
	return
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
	println("Sia Client v" + VERSION)
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Client v" + VERSION,
		Long:  "Sia Client v" + VERSION,
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

	root.AddCommand(minerCmd)
	minerCmd.AddCommand(minerStartCmd, minerStopCmd, minerStatusCmd)

	root.AddCommand(walletCmd)
	walletCmd.AddCommand(walletAddressCmd, walletSendCmd, walletStatusCmd)

	root.AddCommand(renterCmd)
	renterCmd.AddCommand(renterUploadCmd, renterDownloadCmd, renterDownloadQueueCmd, renterStatusCmd)

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
