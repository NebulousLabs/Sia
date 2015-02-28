package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"
)

var (
	APIPort int = 9020
	RPCPort int = 9120
)

type daemonTester struct {
	*daemon
	*testing.T
}

func newDaemonTester(t *testing.T) *daemonTester {
	dc := DaemonConfig{
		APIAddr: ":" + strconv.Itoa(APIPort),
		RPCAddr: ":" + strconv.Itoa(RPCPort),

		HostDir: "hostDir",

		Threads: 1,

		DownloadDir: "downloadDir",

		WalletDir: "walletDir",
	}

	d, err := newDaemon(dc)
	if err != nil {
		t.Fatal("Could not create daemon:", err)
	}
	go func() {
		listenErr := d.listen()
		if listenErr != nil {
			t.Fatal("API server quit:", listenErr)
		}
	}()
	APIPort++
	RPCPort++

	return &daemonTester{d, t}
}

// get wraps a GET request with a status code check, such that if the GET does
// not return 200, the error will be read and returned. The response body is
// not closed.
func (dt *daemonTester) get(call string) (resp *http.Response) {
	resp, err := http.Get("http://localhost" + dt.apiServer.Addr + call)
	if err != nil {
		dt.Fatalf("GET %s failed: %v", call, err)
	}
	// check error code
	if resp.StatusCode != http.StatusOK {
		errResp, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		dt.Fatalf("GET %s returned error %v: %s", call, resp.StatusCode, errResp)
	}
	return
}

// getAPI makes an API call and decodes the response.
func (dt *daemonTester) getAPI(call string, obj interface{}) {
	resp := dt.get(call)
	defer resp.Body.Close()
	err := json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		dt.Fatalf("Could not decode API response: %s", call)
	}
	return
}

// callAPI makes an API call and discards the response.
func (dt *daemonTester) callAPI(call string) {
	dt.get(call).Body.Close()
}

// TestCreateDaemon creates a daemonTester and immediately stops it.
func TestCreateDaemon(t *testing.T) {
	newDaemonTester(t).callAPI("/stop")
}
