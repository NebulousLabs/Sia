package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

var (
	APIPort int = 9020
	RPCPort int = 9120
)

type serverTester struct {
	*Server
	*testing.T
}

func newServerTester(t *testing.T) *serverTester {
	// create testing directory structure
	testdir, err := ioutil.TempDir("..", "testdir")
	if err != nil {
		t.Fatal("Could not create testing dir:", err)
	}

	APIAddr := ":" + strconv.Itoa(APIPort)
	RPCAddr := ":" + strconv.Itoa(RPCPort)
	APIPort++
	RPCPort++

	// create modules
	state := consensus.CreateGenesisState()
	gateway, err := gateway.New(RPCAddr, state, filepath.Join(testdir, "gateway"))
	if err != nil {
		t.Fatal("Failed to create gateway:", err)
	}
	tpool, err := transactionpool.New(state, gateway)
	if err != nil {
		t.Fatal("Failed to create tpool:", err)
	}
	wallet, err := wallet.New(state, tpool, filepath.Join(testdir, "wallet"))
	if err != nil {
		t.Fatal("Failed to create wallet:", err)
	}
	miner, err := miner.New(state, gateway, tpool, wallet)
	if err != nil {
		t.Fatal("Failed to create miner:", err)
	}
	host, err := host.New(state, tpool, wallet, filepath.Join(testdir, "host"))
	if err != nil {
		t.Fatal("Failed to create host:", err)
	}
	hostdb, err := hostdb.New(state, gateway)
	if err != nil {
		t.Fatal("Failed to create hostdb:", err)
	}
	renter, err := renter.New(state, gateway, hostdb, wallet, filepath.Join(testdir, "renter"))
	if err != nil {
		t.Fatal("Failed to create renter:", err)
	}

	srv := NewServer(APIAddr, state, gateway, host, hostdb, miner, renter, tpool, wallet)
	st := &serverTester{srv, t}

	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			t.Fatal("API server quit:", listenErr)
		}
	}()

	// Give the server some money.
	st.mineMoney()

	return st
}

// netAddress returns the NetAddress of the caller.
func (st *serverTester) netAddress() modules.NetAddress {
	return st.gateway.Info().Address
}

// coinAddress returns a coin address that the caller is able to spend from.
func (st *serverTester) coinAddress() string {
	var addr struct {
		Address string
	}
	st.getAPI("/wallet/address", &addr)
	return addr.Address
}

// mineBlock mines a block and puts it into the consensus set.
func (st *serverTester) mineBlock() {
	for {
		_, solved, err := st.miner.FindBlock()
		if err != nil {
			st.Fatal("Mining failed:", err)
		} else if solved {
			// SolveBlock automatically puts the block into the consensus set.
			break
		}
	}
}

// mineMoney mines 5 blocks, enough for the coinbase to be accepted by the
// wallet.
func (st *serverTester) mineMoney() {
	// Get old balance.
	var info modules.WalletInfo
	st.getAPI("/wallet/status", &info)

	// Mine enough blocks to overcome the maturity delay and receive coins.
	for i := types.BlockHeight(0); i < 1+types.MaturityDelay; i++ {
		st.mineBlock()
		st.updateWait()
	}

	// Compare new balance to old balance.
	var info2 modules.WalletInfo
	st.getAPI("/wallet/status", &info2)
	if info2.Balance.Cmp(info.Balance) <= 0 {
		st.Fatal("Mining did not increase balance")
	}
}

// get wraps a GET request with a status code check, such that if the GET does
// not return 200, the error will be read and returned. The response body is
// not closed.
func (st *serverTester) get(call string) (resp *http.Response) {
	resp, err := http.Get("http://localhost" + st.apiServer.Addr + call)
	if err != nil {
		st.Fatalf("GET %s failed: %v", call, err)
	}
	// check error code
	if resp.StatusCode != http.StatusOK {
		errResp, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		st.Fatalf("GET %s returned error %v: %s", call, resp.StatusCode, errResp)
	}
	return
}

// getAPI makes an API call and decodes the response.
func (st *serverTester) getAPI(call string, obj interface{}) {
	resp := st.get(call)
	defer resp.Body.Close()
	err := json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		st.Fatalf("Could not decode API response: %s", call)
	}
	return
}

// callAPI makes an API call and discards the response.
func (st *serverTester) callAPI(call string) {
	st.get(call).Body.Close()
}

// TestCreateServer creates a serverTester and immediately stops it.
func TestCreateServer(t *testing.T) {
	newServerTester(t).callAPI("/daemon/stop")
}
