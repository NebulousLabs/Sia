package api

import (
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

// TestConsensusGet probes the GET call to /consensus.
func TestIntegrationConsensusGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	var cg ConsensusGET
	err = st.getAPI("/consensus", &cg)
	if err != nil {
		t.Fatal(err)
	}
	if cg.Height != 4 {
		t.Error("wrong height returned in consensus GET call")
	}
	if cg.CurrentBlock != st.server.api.cs.CurrentBlock().ID() {
		t.Error("wrong block returned in consensus GET call")
	}
	expectedTarget := types.Target{128}
	if cg.Target != expectedTarget {
		t.Error("wrong target returned in consensus GET call")
	}
}

// TestConsensusValidateTransactionSet probes the POST call to
// /consensus/validate/transactionset.
func TestConsensusValidateTransactionSet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}
	// Block until the allowance has finished forming contracts.
	err = build.Retry(50, time.Millisecond*250, func() error {
		var rc RenterContracts
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 1 {
			return errors.New("no contracts")
		}
		return nil
	})
	if err != nil {
		t.Fatal("allowance setting failed")
	}

	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Get the contract
	var cs RenterContracts
	if err = st.getAPI("/renter/contracts", &cs); err != nil {
		t.Fatal(err)
	}
	if len(cs.Contracts) != 1 {
		t.Fatalf("expected renter to have 1 contracts; got %v", len(cs.Contracts))
	}
	contract := cs.Contracts[0]

	// Validate the contract
	jsonTxns, err := json.Marshal([]types.Transaction{contract.LastTransaction})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := HttpPOST("http://"+st.server.listener.Addr().String()+"/consensus/validate/transactionset", string(jsonTxns))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if non2xx(resp.StatusCode) {
		t.Fatal(decodeError(resp))
	}

	// Try again with an invalid contract
	contract.LastTransaction.FileContractRevisions[0].NewFileSize++
	jsonTxns, err = json.Marshal([]types.Transaction{contract.LastTransaction})
	if err != nil {
		t.Fatal(err)
	}
	resp, err = HttpPOST("http://"+st.server.listener.Addr().String()+"/consensus/validate/transactionset", string(jsonTxns))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !non2xx(resp.StatusCode) {
		t.Fatal("expected validation error")
	}
}
