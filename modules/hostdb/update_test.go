package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestFindHostAnnouncements probes the findHostAnnouncements function
func TestFindHostAnnouncements(t *testing.T) {
	// Create a block with a host announcement.
	announcement := modules.PrefixHostAnnouncement + string(encoding.Marshal(modules.HostAnnouncement{}))
	b := types.Block{
		Transactions: []types.Transaction{
			types.Transaction{
				ArbitraryData: []string{announcement},
			},
		},
	}
	announcements := findHostAnnouncements(b)
	if len(announcements) != 1 {
		t.Error("host announcement not found in block")
	}

	// Try with an altered prefix
	b.Transactions[0].ArbitraryData[0] = "bad" + b.Transactions[0].ArbitraryData[0]
	announcements = findHostAnnouncements(b)
	if len(announcements) != 0 {
		t.Error("host announcement found when there was an invalid prefix")
	}

	// Try with an invalid host encoding.
	b.Transactions[0].ArbitraryData[0] = modules.PrefixHostAnnouncement + "bad"
	announcements = findHostAnnouncements(b)
	if len(announcements) != 0 {
		t.Error("host announcement found when there was an invalid encoding of a host announcement")
	}
}

// TestReceiveConsensusSetUpdate probes teh ReveiveConsensusSetUpdate method of
// the hostdb type.
func TestReceiveConsensusSetUpdate(t *testing.T) {
	hdbt := newHDBTester("TestFindHostAnnouncements", t)

	// Put a host announcement into the blockchain.
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: hdbt.gateway.Info().Address,
	})
	id, err := hdbt.wallet.RegisterTransaction(types.Transaction{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = hdbt.wallet.AddArbitraryData(id, modules.PrefixHostAnnouncement+string(announcement))
	if err != nil {
		t.Fatal(err)
	}
	txn, err := hdbt.wallet.SignTransaction(id, true)
	if err != nil {
		t.Fatal(err)
	}
	err = hdbt.tpool.AcceptTransaction(txn)
	if err != nil {
		t.Fatal(err)
	}
	hdbt.tpUpdateWait()

	// Check that, prior to mining, the hostdb has no hosts.
	if len(hdbt.hostdb.allHosts) != 0 {
		t.Fatal("Hostdb should not yet have any hosts")
	}

	// Mine a block to get the transaction into the consensus set.
	_, _, err = hdbt.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	hdbt.csUpdateWait()

	// Check that there is now a host in the hostdb.
	if len(hdbt.hostdb.allHosts) != 1 {
		t.Fatal("hostdb should have a host after getting a host announcement transcation")
	}
}
