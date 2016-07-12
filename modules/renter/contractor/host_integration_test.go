package contractor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	modWallet "github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// newTestingWallet is a helper function that creates a ready-to-use wallet
// and mines some coins into it.
func newTestingWallet(testdir string, cs modules.ConsensusSet, tp modules.TransactionPool) (modules.Wallet, error) {
	w, err := modWallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key, err := crypto.GenerateTwofishKey()
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
	err = h.AddStorageFolder(storageFolder, 1e6)
	if err != nil {
		return nil, err
	}

	return h, nil
}

// newTestingContractor is a helper function that creates a ready-to-use
// contractor.
func newTestingContractor(testdir string, cs modules.ConsensusSet, tp modules.TransactionPool) (*Contractor, error) {
	w, err := newTestingWallet(testdir, cs, tp)
	if err != nil {
		return nil, err
	}
	hdb, err := hostdb.New(cs, filepath.Join(testdir, "hostdb"))
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
	g, err := gateway.New("localhost:0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, nil, nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
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
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, nil, nil, err
	}
	if !w.Encrypted() {
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
		return nil, nil, nil, err
	}
	c, err := newTestingContractor(filepath.Join(testdir, "Contractor"), cs, tp)
	if err != nil {
		return nil, nil, nil, err
	}

	// announce the host
	err = h.Announce()
	if err != nil {
		return nil, nil, nil, err
	}

	// mine a block, processing the announcement
	m.AddBlock()

	// wait for hostdb to scan host
	for i := 0; i < 500 && len(c.hdb.RandomHosts(1, nil)) == 0; i++ {
		time.Sleep(time.Millisecond)
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
	h, c, _, err := newTestingTrio("TestIntegrationFormContract")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	if contract.NetAddress != h.ExternalSettings().NetAddress {
		t.Fatal("bad contract")
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
	h, c, _, err := newTestingTrio("TestIntegrationReviseContract")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
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
	h, c, _, err := newTestingTrio("TestIntegrationUploadDownload")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
	root, err := editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// download the data
	contract = c.contracts[contract.ID]
	downloader, err := c.Downloader(contract)
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

// TestIntegrationDelete tests that the contractor can delete a sector from a
// contract previously formed with a host.
func TestIntegrationDelete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio("TestIntegrationDelete")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
	_, err = editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// delete the sector
	contract = c.contracts[contract.ID]
	editor, err = c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Delete(contract.MerkleRoots[0])
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationInsertDelete tests that the contractor can insert and delete
// a sector during the same revision.
func TestIntegrationInsertDelete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio("TestIntegrationInsertDelete")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
	// insert the sector
	_, err = editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	// delete the sector
	err = editor.Delete(crypto.MerkleRoot(data))
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// contract should have no sectors
	contract = c.contracts[contract.ID]
	if len(contract.MerkleRoots) != 0 {
		t.Fatal("contract should have no sectors:", contract.MerkleRoots)
	}
}

// TestIntegrationModify tests that the contractor can modify a previously-
// uploaded sector.
func TestIntegrationModify(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio("TestIntegrationModify")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
	// insert the sector
	_, err = editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// modify the sector
	oldRoot := crypto.MerkleRoot(data)
	offset, newData := uint64(10), []byte{1, 2, 3, 4, 5}
	copy(data[offset:], newData)
	newRoot := crypto.MerkleRoot(data)
	contract = c.contracts[contract.ID]
	editor, err = c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Modify(oldRoot, newRoot, offset, newData)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
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
	h, c, _, err := newTestingTrio("TestIntegrationRenew")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
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
	oldContract := c.contracts[contract.ID]
	contract, err = c.managedRenew(oldContract, modules.SectorSize*10, c.blockHeight+200)
	if err != nil {
		t.Fatal(err)
	}

	// check renewed contract
	if contract.FileContract.FileMerkleRoot != root {
		t.Fatal(contract.FileContract.FileMerkleRoot)
	} else if contract.FileContract.FileSize != modules.SectorSize {
		t.Fatal(contract.FileContract.FileSize)
	} else if contract.FileContract.RevisionNumber != 0 {
		t.Fatal(contract.FileContract.RevisionNumber)
	} else if contract.FileContract.WindowStart != c.blockHeight+200 {
		t.Fatal(contract.FileContract.WindowStart)
	}
	// check that Merkle roots are intact
	if len(contract.MerkleRoots) != len(oldContract.MerkleRoots) {
		t.Fatal(len(contract.MerkleRoots), len(oldContract.MerkleRoots))
	}

	// download the renewed contract
	downloader, err := c.Downloader(contract)
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
	oldContract = c.contracts[contract.ID]
	contract, err = c.managedRenew(oldContract, modules.SectorSize*10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}
	if contract.FileContract.WindowStart != c.blockHeight+100 {
		t.Fatal(contract.FileContract.WindowStart)
	}
	// check that Merkle roots are intact
	if len(contract.MerkleRoots) != len(oldContract.MerkleRoots) {
		t.Fatal(len(contract.MerkleRoots), len(oldContract.MerkleRoots))
	}

	// revise the contract
	editor, err = c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err = crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
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

// TestResync tests that the contractor can resync with a host after being
// interrupted during contract revision.
func TestResync(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create testing trio
	h, c, _, err := newTestingTrio("TestResync")
	if err != nil {
		t.Fatal(err)
	}

	// get the host's entry from the db
	hostEntry, ok := c.hdb.Host(h.ExternalSettings().NetAddress)
	if !ok {
		t.Fatal("no entry for host in db")
	}

	// form a contract with the host
	contract, err := c.managedNewContract(hostEntry, 10, c.blockHeight+100)
	if err != nil {
		t.Fatal(err)
	}

	// revise the contract
	editor, err := c.Editor(contract)
	if err != nil {
		t.Fatal(err)
	}
	data, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		t.Fatal(err)
	}
	root, err := editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	err = editor.Close()
	if err != nil {
		t.Fatal(err)
	}

	// download the data
	contract = c.contracts[contract.ID]
	downloader, err := c.Downloader(contract)
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
	contract = c.contracts[contract.ID]

	// corrupt contract and delete its cachedRevision
	badContract := contract
	badContract.LastRevision.NewRevisionNumber--
	badContract.LastRevisionTxn.TransactionSignatures = nil // delete signatures

	c.mu.Lock()
	delete(c.cachedRevisions, contract.ID)
	c.mu.Unlock()

	// Editor and Downloader should fail with the bad contract
	_, err = c.Editor(badContract)
	if !proto.IsRevisionMismatch(err) {
		t.Fatal("expected revision mismatch, got", err)
	}
	_, err = c.Downloader(badContract)
	if !proto.IsRevisionMismatch(err) {
		t.Fatal("expected revision mismatch, got", err)
	}

	// add cachedRevision
	cachedRev := cachedRevision{contract.LastRevision, contract.MerkleRoots}
	c.mu.Lock()
	c.cachedRevisions[contract.ID] = cachedRev
	c.mu.Unlock()

	// Editor and Downloader should now succeed after loading the cachedRevision
	editor, err = c.Editor(badContract)
	if err != nil {
		t.Fatal(err)
	}
	editor.Close()

	downloader, err = c.Downloader(badContract)
	if err != nil {
		t.Fatal(err)
	}
	downloader.Close()

	// corrupt contract and delete its cachedRevision
	badContract = contract
	badContract.LastRevision.NewRevisionNumber--
	badContract.MerkleRoots = nil // delete Merkle roots

	c.mu.Lock()
	delete(c.cachedRevisions, contract.ID)
	c.mu.Unlock()

	// Editor and Downloader should fail with the bad contract
	_, err = c.Editor(badContract)
	if !proto.IsRevisionMismatch(err) {
		t.Fatal("expected revision mismatch, got", err)
	}
	_, err = c.Downloader(badContract)
	if !proto.IsRevisionMismatch(err) {
		t.Fatal("expected revision mismatch, got", err)
	}

	// add cachedRevision
	c.mu.Lock()
	c.cachedRevisions[contract.ID] = cachedRev
	c.mu.Unlock()

	// should be able to upload after loading the cachedRevision
	editor, err = c.Editor(badContract)
	if err != nil {
		t.Fatal(err)
	}
	_, err = editor.Upload(data)
	if err != nil {
		t.Fatal(err)
	}
	editor.Close()
}
