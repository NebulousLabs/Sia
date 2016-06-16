package api

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/explorer"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// serverTester contains a server and a set of channels for keeping all of the
// modules synchronized during testing.
type serverTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	host      modules.Host
	miner     modules.TestMiner
	renter    modules.Renter
	tpool     modules.TransactionPool
	explorer  modules.Explorer
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	server *Server

	dir string
}

// assembleServerTester creates a bunch of modules and assembles them into a
// server tester, without creating any directories or mining any blocks.
func assembleServerTester(key crypto.TwofishKey, testdir string) (*serverTester, error) {
	// assembleServerTester should not get called during short tests, as it
	// takes a long time to run.
	if testing.Short() {
		panic("assembleServerTester called during short tests")
	}

	// Create the modules.
	g, err := gateway.New("localhost:0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	if !w.Encrypted() {
		_, err = w.Encrypt(key)
		if err != nil {
			return nil, err
		}
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	h, err := host.New(cs, tp, w, "localhost:0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}
	r, err := renter.New(cs, w, tp, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}
	e, err := explorer.New(cs, filepath.Join(testdir, modules.ExplorerDir))
	if err != nil {
		return nil, err
	}
	srv, err := NewServer("localhost:0", "Sia-Agent", "", cs, e, g, h, m, r, tp, w)
	if err != nil {
		return nil, err
	}

	// Assemble the serverTester.
	st := &serverTester{
		cs:        cs,
		gateway:   g,
		host:      h,
		miner:     m,
		renter:    r,
		tpool:     tp,
		explorer:  e,
		wallet:    w,
		walletKey: key,

		server: srv,

		dir: testdir,
	}

	// TODO: A more reasonable way of listening for server errors.
	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			panic(listenErr)
		}
	}()
	return st, nil
}

// assembleAuthenticatedServerTester creates a bunch of modules and assembles
// them into a server tester that requires authentication with the given
// requiredPassword. No directories are created and no blocks are mined.
func assembleAuthenticatedServerTester(requiredPassword string, key crypto.TwofishKey, testdir string) (*serverTester, error) {
	// assembleAuthenticatedServerTester should not get called during short
	// tests, as it takes a long time to run.
	if testing.Short() {
		panic("assembleServerTester called during short tests")
	}

	// Create the modules.
	g, err := gateway.New("localhost:0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	if !w.Encrypted() {
		_, err = w.Encrypt(key)
		if err != nil {
			return nil, err
		}
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	h, err := host.New(cs, tp, w, "localhost:0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}
	r, err := renter.New(cs, w, tp, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}
	e, err := explorer.New(cs, filepath.Join(testdir, modules.ExplorerDir))
	if err != nil {
		return nil, err
	}
	srv, err := NewServer("localhost:0", "Sia-Agent", requiredPassword, cs, e, g, h, m, r, tp, w)
	if err != nil {
		return nil, err
	}

	// Assemble the serverTester.
	st := &serverTester{
		cs:        cs,
		gateway:   g,
		host:      h,
		miner:     m,
		renter:    r,
		tpool:     tp,
		explorer:  e,
		wallet:    w,
		walletKey: key,

		server: srv,

		dir: testdir,
	}

	// TODO: A more reasonable way of listening for server errors.
	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			panic(listenErr)
		}
	}()
	return st, nil
}

// assembleExplorerServerTester creates all the explorer dependencies and
// explorer module without creating any directories. The user agent requirement
// is disabled.
func assembleExplorerServerTester(testdir string) (*serverTester, error) {
	// assembleExplorerServerTester should not get called during short tests,
	// as it takes a long time to run.
	if testing.Short() {
		panic("assembleServerTester called during short tests")
	}

	// Create the modules.
	g, err := gateway.New("localhost:0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	e, err := explorer.New(cs, filepath.Join(testdir, modules.ExplorerDir))
	if err != nil {
		return nil, err
	}
	srv, err := NewServer("localhost:0", "", "", cs, e, g, nil, nil, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	// Assemble the serverTester.
	st := &serverTester{
		cs:       cs,
		explorer: e,
		gateway:  g,

		server: srv,

		dir: testdir,
	}

	// TODO: A more reasonable way of listening for server errors.
	go func() {
		listenErr := srv.Serve()
		if listenErr != nil {
			panic(listenErr)
		}
	}()
	return st, nil
}

// createServerTester creates a server tester object that is ready for testing,
// including money in the wallet and all modules initialized.
func createServerTester(name string) (*serverTester, error) {
	// createServerTester is expensive, and therefore should not be called
	// during short tests.
	if testing.Short() {
		panic("createServerTester called during short tests")
	}

	// Create the testing directory.
	testdir := build.TempDir("api", name)

	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	st, err := assembleServerTester(key, testdir)
	if err != nil {
		return nil, err
	}

	// Mine blocks until the wallet has confirmed money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}

	return st, nil
}

// createAuthenticatedServerTester creates an authenticated server tester
// object that is ready for testing, including money in the wallet and all
// modules initalized.
func createAuthenticatedServerTester(name string, password string) (*serverTester, error) {
	// createAuthenticatedServerTester should not get called during short
	// tests, as it takes a long time to run.
	if testing.Short() {
		panic("assembleServerTester called during short tests")
	}

	// Create the testing directory.
	testdir := build.TempDir("authenticated-api", name)

	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	st, err := assembleAuthenticatedServerTester(password, key, testdir)
	if err != nil {
		return nil, err
	}

	// Mine blocks until the wallet has confirmed money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}

	return st, nil
}

// createExplorerServerTester creates a server tester object containing only
// the explorer and some presets that match standard explorer setups.
func createExplorerServerTester(name string) (*serverTester, error) {
	testdir := build.TempDir("api", name)
	st, err := assembleExplorerServerTester(testdir)
	if err != nil {
		return nil, err
	}
	return st, nil
}

// reloadedServerTester creates a server tester where all of the persistent
// data has been copied to a new folder and all of the modules re-initialized
// on the new folder. This gives an opportunity to see how modules will behave
// when they are relying on their persistent structures.
func (st *serverTester) reloadedServerTester() (*serverTester, error) {
	// Copy the testing directory.
	copiedDir := st.dir + " - " + persist.RandomSuffix()
	err := build.CopyDir(st.dir, copiedDir)
	if err != nil {
		return nil, err
	}
	copyST, err := assembleServerTester(st.walletKey, copiedDir)
	if err != nil {
		return nil, err
	}
	return copyST, nil
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

// acceptContracts instructs the host to begin accepting contracts.
func (st *serverTester) acceptContracts() error {
	settingsValues := url.Values{}
	settingsValues.Set("acceptingcontracts", "true")
	return st.stdPostAPI("/host", settingsValues)
}

// setHostStorage adds a 1 GB folder to the host.
func (st *serverTester) setHostStorage() error {
	values := url.Values{}
	values.Set("path", st.dir)
	values.Set("size", "1048576")
	return st.stdPostAPI("/storage/folders/add", values)
}

// announceHost announces the host, mines a block, and waits for the
// announcement to register.
func (st *serverTester) announceHost() error {
	announceValues := url.Values{}
	announceValues.Set("address", string(st.host.ExternalSettings().NetAddress))
	err := st.stdPostAPI("/host/announce", announceValues)
	if err != nil {
		return err
	}
	// mine block
	_, err = st.miner.AddBlock()
	if err != nil {
		return err
	}
	// wait for announcement
	var hosts ActiveHosts
	err = st.getAPI("/renter/hosts/active", &hosts)
	if err != nil {
		return err
	}
	for i := 0; i < 20 && len(hosts.Hosts) == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		err = st.getAPI("/renter/hosts/active", &hosts)
		if err != nil {
			return err
		}
	}
	if len(hosts.Hosts) == 0 {
		return errors.New("host announcement not seen")
	}
	return nil
}

// getAPI makes an API call and decodes the response.
func (st *serverTester) getAPI(call string, obj interface{}) error {
	resp, err := HttpGET("http://" + st.server.listener.Addr().String() + call)
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

// postAPI makes an API call and decodes the response.
func (st *serverTester) postAPI(call string, values url.Values, obj interface{}) error {
	resp, err := HttpPOST("http://"+st.server.listener.Addr().String()+call, values.Encode())
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

// stdGetAPI makes an API call and discards the response.
func (st *serverTester) stdGetAPI(call string) error {
	resp, err := HttpGET("http://" + st.server.listener.Addr().String() + call)
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

// stdGetAPIUA makes an API call with a custom user agent.
func (st *serverTester) stdGetAPIUA(call string, userAgent string) error {
	req, err := http.NewRequest("GET", "http://"+st.server.listener.Addr().String()+call, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := new(http.Client).Do(req)
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

// stdPostAPI makes an API call and discards the response.
func (st *serverTester) stdPostAPI(call string, values url.Values) error {
	resp, err := HttpPOST("http://"+st.server.listener.Addr().String()+call, values.Encode())
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
