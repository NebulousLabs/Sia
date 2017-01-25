package hostdb

import (
	"crypto/rand"
	"io"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// fakeAddr returns a modules.NetAddress to be used in a HostEntry. Such
// addresses are needed in order to satisfy the HostDB's "1 host per IP" rule.
func fakeAddr(n uint8) modules.NetAddress {
	return modules.NetAddress("127.0.0." + strconv.Itoa(int(n)) + ":1")
}

// makeHostDBEntry makes a new host entry with a random public key
func makeHostDBEntry() modules.HostDBEntry {
	dbe := modules.HostDBEntry{}
	pk := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       make([]byte, 32),
	}
	_, err := io.ReadFull(rand.Reader, pk.Key)
	if err != nil {
		panic(err)
	}

	dbe.AcceptingContracts = true
	dbe.PublicKey = pk

	return dbe
}

// TestAverageContractPrice tests the AverageContractPrice method, which also depends on the
// randomHosts method.
func TestAverageContractPrice(t *testing.T) {
	hdb := bareHostDB()

	// empty
	if avg := hdb.AverageContractPrice(); !avg.IsZero() {
		t.Error("average of empty hostdb should be zero:", avg)
	}

	// with one host
	h1 := makeHostDBEntry()
	h1.ContractPrice = types.NewCurrency64(100)
	hdb.hostTree.Insert(h1)
	if avg := hdb.AverageContractPrice(); avg.Cmp(h1.ContractPrice) != 0 {
		t.Error("average of one host should be that host's price:", avg)
	}

	// with two hosts
	h2 := makeHostDBEntry()
	h2.ContractPrice = types.NewCurrency64(300)
	hdb.hostTree.Insert(h2)
	if avg := hdb.AverageContractPrice(); avg.Cmp64(200) != 0 {
		t.Error("average of two hosts should be their sum/2:", avg)
	}
}
