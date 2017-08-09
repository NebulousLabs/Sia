package contractor

import (
	"bytes"
	"os"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// memPersist implements the persister interface in-memory.
type memPersist contractorPersist

func (m *memPersist) save(data contractorPersist) error { *m = memPersist(data); return nil }
func (m *memPersist) update(...journalUpdate) error     { return nil }
func (m memPersist) load(data *contractorPersist) error { *data = contractorPersist(m); return nil }
func (m memPersist) Close() error                       { return nil }

// TestSaveLoad tests that the contractor can save and load itself.
func TestSaveLoad(t *testing.T) {
	// create contractor with mocked persist dependency
	c := &Contractor{
		persist: new(memPersist),
	}

	// add some fake contracts
	c.contracts = map[types.FileContractID]modules.RenterContract{
		{0}: {ID: types.FileContractID{0}, HostPublicKey: types.SiaPublicKey{Key: []byte("foo")}},
		{1}: {ID: types.FileContractID{1}, HostPublicKey: types.SiaPublicKey{Key: []byte("bar")}},
		{2}: {ID: types.FileContractID{2}, HostPublicKey: types.SiaPublicKey{Key: []byte("baz")}},
	}
	c.renewedIDs = map[types.FileContractID]types.FileContractID{
		{0}: {1},
		{1}: {2},
		{2}: {3},
	}
	c.cachedRevisions = map[types.FileContractID]cachedRevision{
		{0}: {Revision: types.FileContractRevision{ParentID: types.FileContractID{0}}},
		{1}: {Revision: types.FileContractRevision{ParentID: types.FileContractID{1}}},
		{2}: {Revision: types.FileContractRevision{ParentID: types.FileContractID{2}}},
	}
	c.oldContracts = map[types.FileContractID]modules.RenterContract{
		{0}: {ID: types.FileContractID{0}, HostPublicKey: types.SiaPublicKey{Key: []byte("foo")}},
		{1}: {ID: types.FileContractID{1}, HostPublicKey: types.SiaPublicKey{Key: []byte("bar")}},
		{2}: {ID: types.FileContractID{2}, HostPublicKey: types.SiaPublicKey{Key: []byte("baz")}},
	}

	// save, clear, and reload
	err := c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.hdb = stubHostDB{}
	c.contracts = make(map[types.FileContractID]modules.RenterContract)
	c.renewedIDs = make(map[types.FileContractID]types.FileContractID)
	c.cachedRevisions = make(map[types.FileContractID]cachedRevision)
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that all fields were restored
	_, ok0 := c.contracts[types.FileContractID{0}]
	_, ok1 := c.contracts[types.FileContractID{1}]
	_, ok2 := c.contracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("contracts were not restored properly:", c.contracts)
	}
	_, ok0 = c.renewedIDs[types.FileContractID{0}]
	_, ok1 = c.renewedIDs[types.FileContractID{1}]
	_, ok2 = c.renewedIDs[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("renewed IDs were not restored properly:", c.renewedIDs)
	}
	_, ok0 = c.cachedRevisions[types.FileContractID{0}]
	_, ok1 = c.cachedRevisions[types.FileContractID{1}]
	_, ok2 = c.cachedRevisions[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("cached revisions were not restored properly:", c.cachedRevisions)
	}
	_, ok0 = c.oldContracts[types.FileContractID{0}]
	_, ok1 = c.oldContracts[types.FileContractID{1}]
	_, ok2 = c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}

	// use stdPersist instead of mock
	c.persist = newPersist(build.TempDir("contractor", t.Name()))
	os.MkdirAll(build.TempDir("contractor", t.Name()), 0700)

	// save, clear, and reload
	err = c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.contracts = make(map[types.FileContractID]modules.RenterContract)
	c.renewedIDs = make(map[types.FileContractID]types.FileContractID)
	c.cachedRevisions = make(map[types.FileContractID]cachedRevision)
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that all fields were restored
	_, ok0 = c.contracts[types.FileContractID{0}]
	_, ok1 = c.contracts[types.FileContractID{1}]
	_, ok2 = c.contracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("contracts were not restored properly:", c.contracts)
	}
	_, ok0 = c.renewedIDs[types.FileContractID{0}]
	_, ok1 = c.renewedIDs[types.FileContractID{1}]
	_, ok2 = c.renewedIDs[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("renewed IDs were not restored properly:", c.renewedIDs)
	}
	_, ok0 = c.cachedRevisions[types.FileContractID{0}]
	_, ok1 = c.cachedRevisions[types.FileContractID{1}]
	_, ok2 = c.cachedRevisions[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("cached revisions were not restored properly:", c.cachedRevisions)
	}
	_, ok0 = c.oldContracts[types.FileContractID{0}]
	_, ok1 = c.oldContracts[types.FileContractID{1}]
	_, ok2 = c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}
}

// blockCS is a consensusSet that calls ProcessConsensusChange on its blocks.
type blockCS struct {
	blocks []types.Block
}

func (cs blockCS) ConsensusSetSubscribe(s modules.ConsensusSetSubscriber, _ modules.ConsensusChangeID, _ <-chan struct{}) error {
	s.ProcessConsensusChange(modules.ConsensusChange{
		AppliedBlocks: cs.blocks,
	})
	return nil
}

func (blockCS) Synced() bool { return true }

func (blockCS) Unsubscribe(modules.ConsensusSetSubscriber) { return }

// TestPubKeyScanner tests that the pubkeyScanner type correctly identifies
// public keys in the blockchain.
func TestPubKeyScanner(t *testing.T) {
	// create pubkeys, announcements, and contracts
	contracts := make(map[types.FileContractID]modules.RenterContract)
	var blocks []types.Block
	var pubkeys []types.SiaPublicKey
	for i := 0; i < 3; i++ {
		// generate a keypair
		sk, pk := crypto.GenerateKeyPair()
		spk := types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       pk[:],
		}
		pubkeys = append(pubkeys, spk)

		// create an announcement and add it to cs
		addr := modules.NetAddress("foo.bar:999" + strconv.Itoa(i))
		ann, err := modules.CreateAnnouncement(addr, spk, sk)
		if err != nil {
			t.Fatal(err)
		}
		blocks = append(blocks, types.Block{
			Transactions: []types.Transaction{{
				ArbitraryData: [][]byte{ann},
			}},
		})

		id := types.FileContractID{byte(i)}
		contracts[id] = modules.RenterContract{ID: id, NetAddress: addr}
	}
	// overwrite the first pubkey with a new one, using the same netaddress.
	// The contractor should use the newer pubkey.
	sk, pk := crypto.GenerateKeyPair()
	spk := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	pubkeys[0] = spk
	ann, err := modules.CreateAnnouncement("foo.bar:9990", spk, sk)
	if err != nil {
		t.Fatal(err)
	}
	blocks = append(blocks, types.Block{
		Transactions: []types.Transaction{{
			ArbitraryData: [][]byte{ann},
		}},
	})

	// create contractor with mocked persist and cs dependencies
	c := &Contractor{
		persist:   new(memPersist),
		cs:        blockCS{blocks},
		contracts: contracts,
	}

	// save, clear, and reload
	err = c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.contracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that contracts were loaded and have their pubkeys filled in
	for i, pk := range pubkeys {
		id := types.FileContractID{byte(i)}
		contract, ok := c.contracts[id]
		if !ok {
			t.Fatal("contracts were not restored properly:", c.contracts)
		}
		// check that pubkey was filled in
		if !bytes.Equal(contract.HostPublicKey.Key, pk.Key) {
			t.Errorf("contract has wrong pubkey: expected %q, got %q", pk.String(), contract.HostPublicKey.String())
		}
	}
}
