package hostdb

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// makeSignedAnnouncement creates a []byte that contains an encoded and signed
// host announcement for the given net address.
func makeSignedAnnouncement(na modules.NetAddress) ([]byte, error) {
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	spk := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	return modules.CreateAnnouncement(na, spk, sk)
}

// TestFindHostAnnouncements probes the findHostAnnouncements function
func TestFindHostAnnouncements(t *testing.T) {
	annBytes, err := makeSignedAnnouncement("foo:1234")
	if err != nil {
		t.Fatal(err)
	}
	b := types.Block{
		Transactions: []types.Transaction{
			types.Transaction{
				ArbitraryData: [][]byte{annBytes},
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
	annBytes, err := makeSignedAnnouncement("foo:1234")
	if err != nil {
		t.Fatal(err)
	}
	cc := modules.ConsensusChange{
		AppliedBlocks: []types.Block{{
			Transactions: []types.Transaction{{
				ArbitraryData: [][]byte{annBytes},
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
