package host

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// announcementFinder is a quick module that parses the blockchain for host
// announcements, keeping a record of all the announcements that get found.
type announcementFinder struct {
	cs modules.ConsensusSet

	announcements []modules.HostAnnouncement
}

// ProcessConsensusChange receives consensus changes from the consensus set and
// parses them for valid host announcements.
func (af *announcementFinder) ProcessConsensusChange(cc modules.ConsensusChange) {
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			for _, arb := range txn.ArbitraryData {
				ann, err := modules.DecodeAnnouncement(arb)
				if err == nil {
					af.announcements = append(af.announcements, ann)
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
	err := cs.ConsensusSetPersistentSubscribe(af, modules.ConsensusChangeID{})
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
	if len(af.announcements) != 1 {
		t.Fatal("could not find host announcement in blockchain")
	}
	if af.announcements[0].NetAddress != ht.host.netAddress {
		t.Error("announcement has wrong address")
	}
	if !bytes.Equal(af.announcements[0].PublicKey.Key, ht.host.publicKey.Key) {
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

	// Create an announcement finder to scan the blockchain for host
	// announcements.
	af, err := newAnnouncementFinder(ht.cs)
	if err != nil {
		t.Fatal(err)
	}
	defer af.Close()

	// Create an announcement, then use the address finding module to scan the
	// blockchain for the host's address.
	addr := modules.NetAddress("foo:1234")
	err = ht.host.AnnounceAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(af.announcements) != 1 {
		t.Fatal("could not find host announcement in blockchain")
	}
	if af.announcements[0].NetAddress != addr {
		t.Error("announcement has wrong address")
	}
	if !bytes.Equal(af.announcements[0].PublicKey.Key, ht.host.publicKey.Key) {
		t.Error("announcement has wrong host key")
	}
}
