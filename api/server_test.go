package api

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/explorer"
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
	// CONTRIBUTE: The naive version of ":0" doesn't work because then the rest
	// of the api tests attempt to call ":0" when making api requests. It would
	// be prefereable to not need to use a port counter and rely on N ports
	// being free starting from 25500 for testing to work.
	APIPort int = 25500
)

// serverTester contains a server and a set of channels for keeping all of the
// modules synchronized during testing.
type serverTester struct {
	cs      *consensus.ConsensusSet
	gateway modules.Gateway
	host    modules.Host
	hostdb  modules.HostDB
	miner   modules.Miner
	renter  modules.Renter
	tpool   modules.TransactionPool
	exp     modules.Explorer
	wallet  modules.Wallet

	server *Server

	t *testing.T
}

// newServerTester creates a server tester object that is ready for testing,
// including money in the wallet and all modules initalized.
func newServerTester(name string, t *testing.T) *serverTester {
	// Create the testing directory and assign the api port.
	testdir := build.TempDir("api", name)
	APIAddr := ":" + strconv.Itoa(APIPort)
	APIPort++

	// Create the modules.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal("Failed to create gateway:", err)
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal("Failed to create consensus set:", err)
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal("Failed to create tpool:", err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal("Failed to create wallet:", err)
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		t.Fatal("Failed to create miner:", err)
	}
	hdb, err := hostdb.New(cs, g)
	if err != nil {
		t.Fatal("Failed to create hostdb:", err)
	}
	h, err := host.New(cs, hdb, tp, w, ":0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		t.Fatal("Failed to create host:", err)
	}
	r, err := renter.New(cs, hdb, w, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		t.Fatal("Failed to create renter:", err)
	}
	exp, err := explorer.New(cs, filepath.Join(testdir, modules.ExplorerDir))
	if err != nil {
		t.Fatal("Failed to create explorer:", err)
	}
	srv, err := NewServer(APIAddr, cs, g, h, hdb, m, r, tp, w, exp)
	if err != nil {
		t.Fatal(err)
	}

	// Assemble the serverTester.
	st := &serverTester{
		cs:      cs,
		gateway: g,
		host:    h,
		hostdb:  hdb,
		miner:   m,
		renter:  r,
		tpool:   tp,
		exp:     exp,
		wallet:  w,

		server: srv,

		t: t,
	}

	// TODO: A more reasonable way of listening for server errors.
	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			t.Fatal("API server quit:", listenErr)
		}
	}()

	// Mine blocks until the wallet has confirmed money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := st.miner.FindBlock()
		err := st.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}

	return st
}

// netAddress returns the NetAddress of the caller.
func (st *serverTester) netAddress() modules.NetAddress {
	return st.server.gateway.Address()
}

// coinAddress returns a coin address that the caller is able to spend from.
func (st *serverTester) coinAddress() string {
	var addr struct {
		Address string
	}
	st.getAPI("/wallet/address", &addr)
	return addr.Address
}

// getAPI makes an API call and decodes the response.
func (st *serverTester) getAPI(call string, obj interface{}) error {
	resp, err := http.Get("http://localhost" + st.server.apiServer.Addr + call)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for a call error.
	if resp.StatusCode != http.StatusOK {
		respErr, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(respErr))
	}

	// Decode the response into 'obj'.
	err = json.NewDecoder(resp.Body).Decode(obj)
	if err != nil {
		return err
	}
	return nil
}

// callAPI makes an API call and discards the response.
func (st *serverTester) callAPI(call string) error {
	resp, err := http.Get("http://localhost" + st.server.apiServer.Addr + call)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for a call error.
	if resp.StatusCode != http.StatusOK {
		respErr, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(respErr))
	}
	return nil
}

// TestCreateServer creates a serverTester and immediately stops it.
func TestCreateServer(t *testing.T) {
	newServerTester("TestCreateServer", t).callAPI("/daemon/stop")
}
