package hostdb

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestFindHostAnnouncements probes the findHostAnnouncements function
func TestFindHostAnnouncements(t *testing.T) {
	// Create a block with a host announcement.
	announcement := append(modules.PrefixHostAnnouncement[:], encoding.Marshal(modules.HostAnnouncement{})...)
	b := types.Block{
		Transactions: []types.Transaction{
			types.Transaction{
				ArbitraryData: [][]byte{announcement},
			},
		},
	}
	announcements := findHostAnnouncements(b)
	if len(announcements) != 1 {
		t.Error("host announcement not found in block")
	}

	// Try with an altered prefix
	b.Transactions[0].ArbitraryData[0][0]++
	announcements = findHostAnnouncements(b)
	if len(announcements) != 0 {
		t.Error("host announcement found when there was an invalid prefix")
	}
	b.Transactions[0].ArbitraryData[0][0]--

	// Try with an invalid host encoding.
	b.Transactions[0].ArbitraryData[0][17]++
	announcements = findHostAnnouncements(b)
	if len(announcements) != 0 {
		t.Error("host announcement found when there was an invalid encoding of a host announcement")
	}
}

// TestReceiveConsensusSetUpdate probes the ReveiveConsensusSetUpdate method of
// the hostdb type.
func TestReceiveConsensusSetUpdate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostDBTester("TestFindHostAnnouncements")
	if err != nil {
		t.Fatal(err)
	}

	// Put a host announcement into the blockchain.
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: ht.gateway.Address(),
	})
	txnBuilder := ht.wallet.StartTransaction()
	txnBuilder.AddArbitraryData(append(modules.PrefixHostAnnouncement[:], announcement...))
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}

	// Check that, prior to mining, the hostdb has no hosts.
	if len(ht.hostdb.AllHosts()) != 0 {
		t.Fatal("Hostdb should not yet have any hosts")
	}

	// Mine a block to get the transaction into the consensus set.
	b, _ := ht.miner.FindBlock()
	err = ht.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Check that there is now a host in the hostdb.
	if len(ht.hostdb.AllHosts()) != 1 {
		t.Fatal("hostdb should have a host after getting a host announcement transcation")
	}
}

// TestIsOffline tests the IsOffline method.
func TestIsOffline(t *testing.T) {
	hdb := &HostDB{
		allHosts: map[modules.NetAddress]*hostEntry{
			"foo:1234": &hostEntry{online: true},
			"bar:1234": &hostEntry{online: false},
			"baz:1234": &hostEntry{online: true},
		},
		activeHosts: map[modules.NetAddress]*hostNode{
			"foo:1234": nil,
		},
		scanPool: make(chan *hostEntry),
	}

	tests := []struct {
		addr    modules.NetAddress
		offline bool
	}{
		{"foo:1234", false},
		{"bar:1234", true},
		{"baz:1234", false},
		{"quux:1234", false},
	}
	for _, test := range tests {
		if offline := hdb.IsOffline(test.addr); offline != test.offline {
			t.Errorf("IsOffline(%v) = %v, expected %v", test.addr, offline, test.offline)
		}
	}

	// quux should have sent host down scanPool
	select {
	case h := <-hdb.scanPool:
		if h.NetAddress != "quux:1234" {
			t.Error("wrong host in scan pool:", h.NetAddress)
		}
	case <-time.After(time.Second):
		t.Error("unknown host was not added to scan pool")
	}
}
