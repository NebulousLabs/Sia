package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/sia"
	"github.com/NebulousLabs/Sia/sia/components"
)

type SuccessResponse struct {
	Success bool
}

// httpReq will request a byte stream from the provided url and log and return
// and errors
func httpReq(t *testing.T, url string) ([]byte, error) {
	resp, err := http.Get("http://127.0.0.1:9980" + url)
	if err != nil {
		t.Log("Could not make http request to " + url)
		return nil, err
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Log("Could not read HTTP response from " + url + " : " + err.Error())
		return nil, err
	}
	if resp.StatusCode != 200 {
		t.Log("HTTP Response from " + url + " returned: " + strconv.Itoa(resp.StatusCode) + string(content))
		return nil, err
	}
	return content, nil
}

// reqJSON will send an http request to the provided url and fill the response
// struct
func reqJSON(t *testing.T, url string, response interface{}) {
	content, err := httpReq(t, url)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = json.Unmarshal(content, &response)
	if err != nil {
		t.Fatal("Could not parse json response to " + url + ": " + err.Error())
	}
	return
}

func reqSuccess(t *testing.T, url string) SuccessResponse {
	var response SuccessResponse
	reqJSON(t, url, &response)
	return response
}

func reqWalletStatus(t *testing.T) components.WalletInfo {
	var r components.WalletInfo
	reqJSON(t, "/wallet/status", r)
	return r
}

func reqHostConfig(t *testing.T) components.HostInfo {
	var r components.HostInfo
	reqJSON(t, "/host/config", &r)
	return r
}

func reqMinerStatus(t *testing.T) components.MinerInfo {
	var r components.MinerInfo
	reqJSON(t, "/miner/status", &r)
	return r
}

func reqWalletAddress(t *testing.T) struct{ Address string } {
	var r struct{ Address string }
	reqJSON(t, "/wallet/address", &r)
	return r
}

func reqGenericStatus(t *testing.T) sia.StateInfo {
	var r sia.StateInfo
	reqJSON(t, "/status", &r)
	return r
}

func reqFileStatus(t *testing.T) components.RentInfo {
	var r components.RentInfo
	reqJSON(t, "/file/status", &r)
	return r
}

func reqHostSetConfig(t *testing.T, hostInfo components.HostInfo) SuccessResponse {
	// return reqSuccess(t, "/host/setconfig")
	var params url.Values
	params.Add("totalstorage", fmt.Sprintf("%d", hostInfo.Announcement.TotalStorage))
	params.Add("maxfilesize", fmt.Sprintf("%d", hostInfo.Announcement.MaxFilesize))
	params.Add("mintolerance", fmt.Sprintf("%d", hostInfo.Announcement.MinTolerance))
	params.Add("maxduration", fmt.Sprintf("%d", hostInfo.Announcement.MaxDuration))
	params.Add("price", fmt.Sprintf("%d", hostInfo.Announcement.Price))
	params.Add("burn", fmt.Sprintf("%d", hostInfo.Announcement.Burn))

	urlWithParams := "http://127.0.0.1:9980/host/setconfig?" + params.Encode()

	resp, err := http.Get(urlWithParams)
	if err != nil {
		t.Fatal("Couldn't set host config: " + err.Error())
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Could not read set host config response: " + err.Error())
	}

	var fResponse SuccessResponse

	err = json.Unmarshal(content, &fResponse)
	if err != nil {
		t.Fatal("Could not parse set host config response: " + err.Error())
	}

	return fResponse
}

func reqMinerStart(t *testing.T) SuccessResponse {
	return reqSuccess(t, "/miner/start")
}

func reqMinerStop(t *testing.T) SuccessResponse {
	return reqSuccess(t, "/miner/stop")
}

// /update/check

// /host/setconfig
// /miner/stop
// /wallet/send
// /file/upload
// /file/download
// /sync
// /peer/add
// /peer/remove
// /update/apply
// /stop

func setupDaemon(t *testing.T) {

	// Settings to speed up operations
	consensus.BlockFrequency = consensus.Timestamp(1)
	consensus.TargetWindow = consensus.BlockHeight(1000)
	network.BootstrapPeers = []network.Address{"localhost:9988"}
	consensus.RootTarget[0] = 255
	consensus.MaxAdjustmentUp = big.NewRat(1005, 1000)
	consensus.MaxAdjustmentDown = big.NewRat(995, 1000)

	var config Config

	config.Siad.ConfigFilename = filepath.Join(siaDir, "config")
	config.Siacore.HostDirectory = filepath.Join(siaDir, "hostdir")
	config.Siad.StyleDirectory = filepath.Join(siaDir, "style")
	config.Siad.DownloadDirectory = "~/Downloads"
	config.Siad.WalletFile = filepath.Join(siaDir, "sia.wallet")
	config.Siad.APIaddr = "localhost:9980"
	config.Siacore.RPCaddr = ":9988"
	config.Siacore.NoBootstrap = false
	err := config.expand()
	if err != nil {
		t.Fatal("Couldn't expand config: " + err.Error())
	}

	go func() {
		err = startDaemon(config)
		if err != nil {
			t.Fatal("Couldn't start daemon: " + err.Error())
		}
	}()

	// Give the daemon time to initialize
	time.Sleep(10 * time.Millisecond)

	// First call is just to see if daemon booted
	_, err = httpReq(t, "/wallet/status")
	if err != nil {
		t.Fatal("Daemon could not handle first request (after 10ms) " + err.Error())
	}
}

// This only works if there is already a daemon running
func TestWalletLockup(t *testing.T) {

	setupDaemon(t)

	// if !testing.Short() {
	//
	// }

}
