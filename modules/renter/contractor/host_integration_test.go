package contractor

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	modWallet "github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// newTestingWallet is a helper function that creates a ready-to-use wallet
// and mines some coins into it.
func newTestingWallet(testdir string, cs modules.ConsensusSet, tp modules.TransactionPool) (modules.Wallet, error) {
	w, err := modWallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key := crypto.GenerateTwofishKey()
	encrypted, err := w.Encrypted()
	if err != nil {
		return nil, err
	}
	if !encrypted {
		_, err = w.Encrypt(key)
		if err != nil {
			return nil, err
		}
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	// give it some money
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := m.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return w, nil
}

// newTestingHost is a helper function that creates a ready-to-use host.
func newTestingHost(testdir string, cs modules.ConsensusSet, tp modules.TransactionPool) (modules.Host, error) {
	w, err := newTestingWallet(testdir, cs, tp)
	if err != nil {
		return nil, err
	}
	h, err := host.New(cs, tp, w, "localhost:0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}

	// configure host to accept contracts
	settings := h.InternalSettings()
	settings.AcceptingContracts = true
	err = h.SetInternalSettings(settings)
	if err != nil {
		return nil, err
	}

	// add storage to host
	storageFolder := filepath.Join(testdir, "storage")
	err = os.MkdirAll(storageFolder, 0700)
	if err != nil {
		return nil, err
	}
	err = h.AddStorageFolder(storageFolder, modules.SectorSize*64)
	if err != nil {
		return nil, err
	}

	return h, nil
}

// newTestingContractor is a helper function that creates a ready-to-use
// contractor.
func newTestingContractor(testdir string, g modules.Gateway, cs modules.ConsensusSet, tp modules.TransactionPool) (*Contractor, error) {
	w, err := newTestingWallet(testdir, cs, tp)
	if err != nil {
		return nil, err
	}
	hdb, err := hostdb.New(g, cs, filepath.Join(testdir, "hostdb"))
	if err != nil {
		return nil, err
	}
	return New(cs, w, tp, hdb, filepath.Join(testdir, "contractor"))
}

// newTestingTrio creates a Host, Contractor, and TestMiner that can be used
// for testing host/renter interactions.
func newTestingTrio(name string) (modules.Host, *Contractor, modules.TestMiner, error) {
	testdir := build.TempDir("contractor", name)

	// create miner
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, nil, nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, nil, nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, nil, nil, err
	}
	w, err := modWallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, nil, nil, err
	}
	key := crypto.GenerateTwofishKey()
	encrypted, err := w.Encrypted()
	if err != nil {
		return nil, nil, nil, err
	}
	if !encrypted {
		_, err = w.Encrypt(key)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, nil, nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, nil, nil, err
	}

	// create host and contractor, using same consensus set and gateway
	h, err := newTestingHost(filepath.Join(testdir, "Host"), cs, tp)
	if err != nil {
		return nil, nil, nil, build.ExtendErr("error creating testing host", err)
	}
	c, err := newTestingContractor(filepath.Join(testdir, "Contractor"), g, cs, tp)
	if err != nil {
		return nil, nil, nil, err
	}

	// announce the host
	err = h.Announce()
	if err != nil {
		return nil, nil, nil, build.ExtendErr("error announcing host", err)
	}

	// mine a block, processing the announcement
	_, err = m.AddBlock()
	if err != nil {
		return nil, nil, nil, err
	}

	// wait for hostdb to scan host
	for i := 0; i < 50 && len(c.hdb.ActiveHosts()) == 0; i++ {
		time.Sleep(time.Millisecond * 100)
	}
	if len(c.hdb.ActiveHosts()) == 0 {
		return nil, nil, nil, errors.New("host did not make it into the contractor hostdb in time")
	}

	return h, c, m, nil
}

// TestIntegrationFormContract tests that the contractor can form contracts
// with the host module.
func TestIntegrationFormContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	h, c, _, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	defer c.Close()

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.PublicKey())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	_, err = c.managedNewContract(hostEntry, types.SiacoinPrecision.Mul64(50), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationReviseContract tests that the contractor can revise a
// contract previously formed with a host.
func TestIntegrationReviseContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	defer c.Close()

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.PublicKey())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, types.SiacoinPrecision.Mul64(50), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := fastrand.Bytes(int(modules.SectorSize))
	_, err = editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationUploadDownload tests that the contractor can upload data to
// a host and download it intact.
func TestIntegrationUploadDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	defer c.Close()

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.PublicKey())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, types.SiacoinPrecision.Mul64(50), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := fastrand.Bytes(int(modules.SectorSize))
	root, err := editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// download the data
	downloader, err := c.Downloader(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := downloader.Sector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, retrieved) {
		t.Fatal("downloaded data does not match original")
	}
	err = downloader.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationRenew tests that the contractor can renew a previously-
// formed file contract.
func TestIntegrationRenew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	defer c.Close()

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.PublicKey())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, types.SiacoinPrecision.Mul64(50), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := fastrand.Bytes(int(modules.SectorSize))
	// insert the sector
	root, err := editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// renew the contract
	err = c.managedUpdateContractUtility(contract.ID, modules.ContractUtility{GoodForRenew: true})
	if err != nil {
		t.Fatal(err)
	}
	oldContract, ok := c.staticContracts.Acquire(contract.ID)
	if !ok {
		t.Fatal("failed to acquire contract")
	}
	contract, err = c.managedRenew(oldContract, types.SiacoinPrecision.Mul64(50), c.blockHeight+200)
	if err != nil {
		t.Fatal(err)
	}
	c.staticContracts.Return(oldContract)

	// check renewed contract
	if contract.EndHeight != c.blockHeight+200 {
		t.Fatal(contract.EndHeight)
	}

	// download the renewed contract
	downloader, err := c.Downloader(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := downloader.Sector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, retrieved) {
		t.Fatal("downloaded data does not match original")
	}
	err = downloader.Close()
	if err != nil {
		t.Fatal(err)
	}

	// renew to a lower height
	err = c.managedUpdateContractUtility(contract.ID, modules.ContractUtility{GoodForRenew: true})
	if err != nil {
		t.Fatal(err)
	}
	oldContract, _ = c.staticContracts.Acquire(contract.ID)
	contract, err = c.managedRenew(oldContract, types.SiacoinPrecision.Mul64(50), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}
	c.staticContracts.Return(oldContract)
	if contract.EndHeight != c.blockHeight+100 {
		t.Fatal(contract.EndHeight)
	}

	// revise the contract
	editor, err = c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	data = fastrand.Bytes(int(modules.SectorSize))
	// insert the sector
	_, err = editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationDownloaderCaching tests that downloaders are properly cached
// by the contractor. When two downloaders are requested for the same
// contract, only one underlying downloader should be created.
func TestIntegrationDownloaderCaching(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	defer c.Close()

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.PublicKey())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, types.SiacoinPrecision.Mul64(50), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// create a downloader
	d1, err := c.Downloader(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// create another downloader
	d2, err := c.Downloader(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// downloaders should match
	if d1 != d2 {
		t.Fatal("downloader was not cached")
	}

	// close one of the downloaders; it should not fully close, since d1 is
	// still using it
	d2.Close()

	c.mu.RLock()
	_, ok = c.downloaders[contract.ID]
	c.mu.RUnlock()
	if !ok {
		t.Fatal("expected downloader to still be present")
	}

	// create another downloader
	d3, err := c.Downloader(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// downloaders should match
	if d3 != d1 {
		t.Fatal("closing one client should not fully close the downloader")
	}

	// close both downloaders
	d1.Close()
	d2.Close()

	c.mu.RLock()
	_, ok = c.downloaders[contract.ID]
	c.mu.RUnlock()
	if ok {
		t.Fatal("did not expect downloader to still be present")
	}

	// create another downloader
	d4, err := c.Downloader(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// downloaders should match
	if d4 == d1 {
		t.Fatal("downloader should not have been cached after all clients were closed")
	}
	d4.Close()
}

// TestIntegrationEditorCaching tests that editors are properly cached
// by the contractor. When two editors are requested for the same
// contract, only one underlying editor should be created.
func TestIntegrationEditorCaching(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	defer c.Close()

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.PublicKey())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, types.SiacoinPrecision.Mul64(50), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// create an editor
	d1, err := c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// create another editor
	d2, err := c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// editors should match
	if d1 != d2 {
		t.Fatal("editor was not cached")
	}

	// close one of the editors; it should not fully close, since d1 is
	// still using it
	d2.Close()

	c.mu.RLock()
	_, ok = c.editors[contract.ID]
	c.mu.RUnlock()
	if !ok {
		t.Fatal("expected editor to still be present")
	}

	// create another editor
	d3, err := c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// editors should match
	if d3 != d1 {
		t.Fatal("closing one client should not fully close the editor")
	}

	// close both editors
	d1.Close()
	d2.Close()

	c.mu.RLock()
	_, ok = c.editors[contract.ID]
	c.mu.RUnlock()
	if ok {
		t.Fatal("did not expect editor to still be present")
	}

	// create another editor
	d4, err := c.Editor(contract.HostPublicKey, nil)
	if err != nil {
		t.Fatal(err)
	}

	// editors should match
	if d4 == d1 {
		t.Fatal("editor should not have been cached after all clients were closed")
	}
	d4.Close()
}

// TestContractPresenceLeak tests that a renter can not tell from the response
// of the host to RPCs if the host has the contract if the renter doesn't
// own this contract. See https://github.com/NebulousLabs/Sia/issues/2327.
func TestContractPresenceLeak(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	defer c.Close()

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.PublicKey())
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, types.SiacoinPrecision.Mul64(10), c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// Connect with bad challenge response. Try correct
	// and incorrect contract IDs. Compare errors.
	wrongID := contract.ID
	wrongID[0] ^= 0x01
	fcids := []types.FileContractID{contract.ID, wrongID}
	var errors []error

	for _, fcid := range fcids {
		var challenge crypto.Hash
		var signature crypto.Signature
		conn, err := net.Dial("tcp", string(hostEntry.NetAddress))
		if err != nil {
			t.Fatalf("Couldn't dial tpc connection with host @ %v: %v.", string(hostEntry.NetAddress), err)
		}
		if err := encoding.WriteObject(conn, modules.RPCDownload); err != nil {
			t.Fatalf("Couldn't initiate RPC: %v.", err)
		}
		if err := encoding.WriteObject(conn, fcid); err != nil {
			t.Fatalf("Couldn't send fcid: %v.", err)
		}
		if err := encoding.ReadObject(conn, &challenge, 32); err != nil {
			t.Fatalf("Couldn't read challenge: %v.", err)
		}
		if err := encoding.WriteObject(conn, signature); err != nil {
			t.Fatalf("Couldn't send signature: %v.", err)
		}
		err = modules.ReadNegotiationAcceptance(conn)
		if err == nil {
			t.Fatal("Expected an error, got success.")
		}
		errors = append(errors, err)
	}
	if errors[0].Error() != errors[1].Error() {
		t.Fatalf("Expected to get equal errors, got %q and %q.", errors[0], errors[1])
	}
}
