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
	ht := CreateHostTester("TestAnnouncement", t)

	// Place the announcement.
	err := ht.host.ForceAnnounce("foo")
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

	// TODO: Need to check that the host announcement gets the host into the
	// hostdb.
}
