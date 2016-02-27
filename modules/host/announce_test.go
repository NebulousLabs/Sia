package host

import (
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestAnnouncement has a host announce itself to the blockchain and then
// checks that the announcement makes it correctly.
func TestAnnouncement(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestAnnouncement")
	if err != nil {
		t.Fatal(err)
	}

	// Place the announcement.
	ht.host.mu.RLock()
	addr := ht.host.netAddress
	ht.host.mu.RUnlock()
	err = ht.host.AnnounceAddress(addr)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the announcement made it into the transaction pool correctly.
	txns := ht.tpool.TransactionList()
	if len(txns) != 1 {
		t.Fatal("Expecting 1 transaction in transaction pool, instead there was", len(txns))
	}
	encodedAnnouncement := txns[0].ArbitraryData[0][types.SpecifierLen:]
	var ha modules.HostAnnouncement
	err = encoding.Unmarshal([]byte(encodedAnnouncement), &ha)
	if err != nil {
		t.Error(err)
	}

	// Mine a block to get the announcement into the blockchain, and then wait
	// until the hostdb recognizes the host.
	/*
		_, err = ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 50 && len(ht.renter.ActiveHosts()) == 0; i++ {
			time.Sleep(time.Millisecond * 50)
		}
		if len(ht.renter.ActiveHosts()) == 0 {
			t.Fatal("no active hosts in hostdb after host made an announcement")
		}
	*/
}
