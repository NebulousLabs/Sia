package hostdb

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestFindHostAnnouncements probes the findHostAnnouncements function
func TestFindHostAnnouncements(t *testing.T) {
	// Create a block with a valid host announcement.
	var emptyKey crypto.PublicKey
	announcement := encoding.MarshalAll(modules.PrefixHostAnnouncement, modules.HostAnnouncement{
		NetAddress: "foo:1234",
		PublicKey: types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       emptyKey[:],
		},
	})
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
	// create hostdb
	hdb := bareHostDB()

	// Put a host announcement into a block.
	var emptyKey crypto.PublicKey
	announceBytes := encoding.MarshalAll(modules.PrefixHostAnnouncement, modules.HostAnnouncement{
		NetAddress: "foo:1234",
		PublicKey: types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       emptyKey[:],
		},
	})
	cc := modules.ConsensusChange{
		AppliedBlocks: []types.Block{{
			Transactions: []types.Transaction{{
				ArbitraryData: [][]byte{announceBytes},
			}},
		}},
	}

	// call ProcessConsensusChange
	hdb.ProcessConsensusChange(cc)

	// host should be sent to scanPool
	select {
	case <-time.After(time.Second):
		t.Fatal("announcement not seen")
	case <-hdb.scanPool:
	}

	// Check that there is now a host in the hostdb.
	if len(hdb.AllHosts()) != 1 {
		t.Fatal("hostdb should have a host after getting a host announcement transcation")
	}
}
