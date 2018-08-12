package host

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// announcementFinder is a quick module that parses the blockchain for host
// announcements, keeping a record of all the announcements that get found.
type announcementFinder struct {
	cs modules.ConsensusSet

	// Announcements that have been seen. The two slices are wedded.
	netAddresses []modules.NetAddress
	publicKeys   []types.SiaPublicKey
}

// ProcessConsensusChange receives consensus changes from the consensus set and
// parses them for valid host announcements.
func (af *announcementFinder) ProcessConsensusChange(cc modules.ConsensusChange) {
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			for _, arb := range txn.ArbitraryData {
				addr, pubKey, err := modules.DecodeAnnouncement(arb)
				if err == nil {
					af.netAddresses = append(af.netAddresses, addr)
					af.publicKeys = append(af.publicKeys, pubKey)
				}
			}
		}
	}
}

// Close will shut down the announcement finder.
func (af *announcementFinder) Close() error {
	af.cs.Unsubscribe(af)
	return nil
}

// newAnnouncementFinder will create and return an announcement finder.
func newAnnouncementFinder(cs modules.ConsensusSet) (*announcementFinder, error) {
	af := &announcementFinder{
		cs: cs,
	}
	err := cs.ConsensusSetSubscribe(af, modules.ConsensusChangeBeginning, nil)
	if err != nil {
		return nil, err
	}
	return af, nil
}

// TestHostAnnounce checks that the host announce function is operating
// correctly.
func TestHostAnnounce(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestHostAnnounce")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Create an announcement finder to scan the blockchain for host
	// announcements.
	af, err := newAnnouncementFinder(ht.cs)
	if err != nil {
		t.Fatal(err)
	}
	defer af.Close()

	// Create an announcement, then use the address finding module to scan the
	// blockchain for the host's address.
	err = ht.host.Announce()
	if err != nil {
		t.Fatal(err)
	}
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(af.publicKeys) != 1 {
		t.Fatal("could not find host announcement in blockchain")
	}
	if af.netAddresses[0] != ht.host.autoAddress {
		t.Error("announcement has wrong address")
	}
	if !bytes.Equal(af.publicKeys[0].Key, ht.host.publicKey.Key) {
		t.Error("announcement has wrong host key")
	}
}

// TestHostAnnounceAddress checks that the host announce address function is
// operating correctly.
func TestHostAnnounceAddress(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestHostAnnounceAddress")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Create an announcement finder to scan the blockchain for host
	// announcements.
	af, err := newAnnouncementFinder(ht.cs)
	if err != nil {
		t.Fatal(err)
	}
	defer af.Close()

	// Create an announcement, then use the address finding module to scan the
	// blockchain for the host's address.
	addr := modules.NetAddress("foo.com:1234")
	err = ht.host.AnnounceAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(af.netAddresses) != 1 {
		t.Fatal("could not find host announcement in blockchain")
	}
	if af.netAddresses[0] != addr {
		t.Error("announcement has wrong address")
	}
	if !bytes.Equal(af.publicKeys[0].Key, ht.host.publicKey.Key) {
		t.Error("announcement has wrong host key")
	}
}

// TestHostAnnounceCheckUnlockHash verifies that the host's unlock hash is
// checked when an announcement is performed.
func TestHostAnnounceCheckUnlockHash(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	ht.host.mu.RLock()
	oldUnlockHash := ht.host.unlockHash
	ht.host.mu.RUnlock()

	err = ht.wallet.Reset()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.initWallet()
	if err != nil {
		t.Fatal(err)
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.Announce()
	if err != nil {
		t.Fatal(err)
	}
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	newUnlockHash := ht.host.unlockHash
	ht.host.mu.RUnlock()
	if newUnlockHash == oldUnlockHash {
		t.Fatal("host did not set a new unlock hash after announce with reset wallet")
	}
	hasAddr := false
	addrs, err := ht.wallet.AllAddresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		if addr == newUnlockHash {
			hasAddr = true
			break
		}
	}
	if !hasAddr {
		t.Fatal("host unlock has did not exist in wallet")
	}
}
