package host

import (
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// TestAnnouncement has a host announce itself to the blockchain and then
// checks that the announcement makes it correctly.
func TestAnnouncement(t *testing.T) {
	ht := CreateHostTester(t)

	// Place the announcement.
	originalAddress := network.Address(":1111")
	err := ht.Announce(originalAddress)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the announcement made it into the transaction pool correctly.
	txns, err := ht.tpool.TransactionSet()
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != 1 {
		t.Error("Expecting 1 transaction in transaction pool, instead there was", len(txns))
	}
	encodedAnnouncement := strings.TrimPrefix(txns[0].ArbitraryData[0], modules.PrefixHostAnnouncement)
	var ha modules.HostAnnouncement
	err = encoding.Unmarshal([]byte(encodedAnnouncement), &ha)
	if err != nil {
		t.Error(err)
	}
	if ha.IPAddress != originalAddress {
		t.Error("announcement didn't decode properly after being put in the transation pool")
	}
}
