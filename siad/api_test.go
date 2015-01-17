package main

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
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

func reqStatus(t *testing.T, url string, responseForm interface{}) {
	content, err := httpReq(t, url)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = json.Unmarshal(content, &responseForm)
	if err != nil {
		t.Fatal("Could not parse json response: " + err.Error())
	}
	return
}

func reqSuccess(t *testing.T, url string) SuccessResponse {
	content, err := httpReq(t, url)
	if err != nil {
		t.Fatal(err.Error())
	}
	var success SuccessResponse
	err = json.Unmarshal(content, &success)
	if err != nil {
		t.Fatal("Could not parse json response: " + err.Error())
	}
	return success
}

func reqWalletStatus(t *testing.T) components.WalletInfo {
	var r components.WalletInfo
	reqStatus(t, "/wallet/status", r)
	return r
}

func reqHostConfig(t *testing.T) components.HostInfo {
	var r components.HostInfo
	reqStatus(t, "/host/config", &r)
	return r
}

func reqMinerStatus(t *testing.T) components.MinerInfo {
	var r components.MinerInfo
	reqStatus(t, "/miner/status", &r)
	return r
}

func reqWalletAddress(t *testing.T) struct{ Address string } {
	var r struct{ Address string }
	reqStatus(t, "/wallet/address", &r)
	return r
}

func reqGenericStatus(t *testing.T) sia.StateInfo {
	var r sia.StateInfo
	reqStatus(t, "/status", &r)
	return r
}

func reqFileStatus(t *testing.T) components.RentInfo {
	var r components.RentInfo
	reqStatus(t, "/file/status", &r)
	return r
}

func reqHostSetConfig(t *testing.T) SuccessResponse {
	// return reqSuccess(t, "/host/setconfig")
}

// /update/check

// /host/setconfig
// /miner/start
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
