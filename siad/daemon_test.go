package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

var (
	APIPort int = 9020
	RPCPort int = 9120
)

type daemonTester struct {
	*daemon
	*testing.T
	rpcChan chan struct{}
}

func newDaemonTester(t *testing.T) *daemonTester {
	dc := DaemonConfig{
		APIAddr: ":" + strconv.Itoa(APIPort),
		RPCAddr: ":" + strconv.Itoa(RPCPort),

		HostDir: "hostDir",

		Threads: 1,

		DownloadDir: "downloadDir",

		WalletDir: "walletDir" + strconv.Itoa(APIPort),
	}

	d, err := newDaemon(dc)
	if err != nil {
		t.Fatal("Could not create daemon:", err)
	}
	dt := &daemonTester{d, t, make(chan struct{})}

	// Overwrite RPCs with special testing RPCs.
	dt.network.RegisterRPC("AddMe", dt.addMe)
	dt.network.RegisterRPC("RelayBlock", dt.relayBlock)
	dt.network.RegisterRPC("AcceptTransaction", dt.acceptTransaction)

	go func() {
		listenErr := d.listen()
		if listenErr != nil {
			t.Fatal("API server quit:", listenErr)
		}
	}()
	APIPort++
	RPCPort++

	// Give the daemon some money.
	// dt.mineMoney()

	return dt
}

// netAddress returns the network address of the caller.
func (dt *daemonTester) netAddress() network.Address {
	return dt.network.Address()
}

// coinAddress returns a coin address that the caller is able to spend from.
func (dt *daemonTester) coinAddress() string {
	var addr struct {
		Address string
	}
	dt.getAPI("/wallet/address", &addr)
	return addr.Address
}

func (dt *daemonTester) addMe(peer network.Address) error {
	err := dt.gateway.AddMe(peer)
	dt.rpcChan <- struct{}{}
	return err
}

func (dt *daemonTester) relayBlock(b consensus.Block) error {
	err := dt.gateway.RelayBlock(b)
	//dt.rpcChan <- struct{}{}
	return err
}

func (dt *daemonTester) acceptTransaction(t consensus.Transaction) error {
	err := dt.tpool.AcceptTransaction(t)
	dt.rpcChan <- struct{}{}
	return err
}

// mineBlock mines a block and puts it into the consensus set.
func (dt *daemonTester) mineBlock() {
	for {
		_, solved, err := dt.miner.SolveBlock()
		if err != nil {
			dt.Fatal("Mining faild:", err)
		} else if solved {
			// SovleBlock automatically puts the block into the consensus set.
			break
		}
	}
}

// mineMoney mines 5 blocks, enough for the coinbase to be accepted by the
// wallet.
func (dt *daemonTester) mineMoney() {
	// Get old balance.
	var info modules.WalletInfo
	dt.getAPI("/wallet/status", &info)
	oldBalance := info.Balance

	// Mine enough blocks to overcome the maturity delay and receive coins.
	for i := 0; i < 1+consensus.MaturityDelay; i++ {
		dt.mineBlock()
	}

	// Compare new balance to old balance.
	var info2 modules.WalletInfo
	dt.getAPI("/wallet/status", &info2)
	if info2.FullBalance.Cmp(oldBalance) <= 0 {
		dt.Fatal("Mining did not increase balance")
	}
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
	newDaemonTester(t).callAPI("/daemon/stop")
}
