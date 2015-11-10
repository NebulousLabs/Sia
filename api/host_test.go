package api

import (
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationHosting tests that the host correctly receives payment for
// hosting files.
func TestIntegrationHosting(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestIntegrationHosting")
	if err != nil {
		t.Fatal(err)
	}

	// announce the host
	err = st.stdGetAPI("/host/announce?address=" + string(st.host.Address()))
	if err != nil {
		t.Fatal(err)
	}
	// we need to announce twice, or the renter will complain about not having enough hosts
	loopAddr := "127.0.0.1:" + st.host.Address().Port()
	err = st.stdGetAPI("/host/announce?address=" + loopAddr)
	if err != nil {
		t.Fatal(err)
	}

	// wait for announcement to register
	st.miner.AddBlock()
	var hosts ActiveHosts
	st.getAPI("/hostdb/hosts/active", &hosts)
	if len(hosts.Hosts) == 0 {
		t.Fatal("host announcement not seen")
	}

	// create a file
	path := filepath.Join(build.SiaTestingDir, "api", "TestIntegrationHosting", "test.dat")
	data, err := crypto.RandBytes(1024)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(path, data, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// upload to host
	err = st.stdGetAPI("/renter/files/upload?nickname=test&duration=10&source=" + path)
	if err != nil {
		t.Fatal(err)
	}
	var fi []FileInfo
	for len(fi) != 1 || fi[0].UploadProgress != 100 {
		st.getAPI("/renter/files/list", &fi)
		time.Sleep(3 * time.Second)
	}

	// mine blocks until storage proof is complete
	for i := 0; i < 20+int(types.MaturityDelay); i++ {
		st.miner.AddBlock()
	}

	// check balance
	var wi WalletGET
	st.getAPI("/wallet", &wi)
	withoutProofBal := "7499794770722000000001457682072"
	if wi.ConfirmedSiacoinBalance.String() == withoutProofBal {
		t.Fatal("host's balance was not affected: expected %v, got %v", withoutProofBal, wi.ConfirmedSiacoinBalance)
	}
}
