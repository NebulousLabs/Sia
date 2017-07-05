package api

import (
	"encoding/json"
	"net/url"
	"testing"

	"fmt"
	"github.com/NebulousLabs/Sia/modules"
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
	st.miner.AddBlock()

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

// TestConsensusChange probes the GET call to /consensus/change.
func TestConsensusChange(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Request the consensus change beginning
	requestedID := modules.ConsensusChangeBeginning
	cc, nextID, err := st.cs.GetConsensusChange(requestedID)

	// Send the GET request
	var ccg ConsensusChangeGet
	err = st.getAPI(fmt.Sprintf("/consensus/change/%x", requestedID[:]), &ccg)
	if err != nil {
		t.Fatal(err)
	}

	// Compare the values of the returned consensus change with the retrieved one
	var replyIDBytes []byte
	var replyNextIDBytes []byte
	var replyID modules.ConsensusChangeID
	var replyNextID modules.ConsensusChangeID

	// Parse the ids of the request
	_, err = fmt.Sscanf(ccg.ID, "%x", &replyIDBytes)
	if err != nil {
		t.Error(err)
	}
	_, err = fmt.Sscanf(ccg.NextID, "%x", &replyNextIDBytes)
	if err != nil {
		t.Error(err)
	}
	copy(replyID[:], replyIDBytes)
	copy(replyNextID[:], replyNextIDBytes)

	// Check if values match
	if replyID != cc.ID {
		t.Errorf("ID mismatch: expected %v but was %v", cc.ID, ccg.ID)
	}
	if replyNextID != nextID {
		t.Errorf("NextID mismatch: expected %v but was %v", nextID, ccg.NextID)
	}

	// Request the next change
	var ccg2 ConsensusChangeGet
	err = st.getAPI("/consensus/change/"+ccg.NextID, &ccg2)
	if err != nil {
		t.Fatal(err)
	}

	// Check if the id of the next change matches the NextID of the previous change
	var replyIDBytes2 []byte
	var replyID2 modules.ConsensusChangeID

	_, err = fmt.Sscanf(ccg2.ID, "%x", &replyIDBytes2)
	if err != nil {
		t.Error(err)
	}

	copy(replyID2[:], replyIDBytes2)
	if replyID2 != replyNextID {
		t.Errorf("ID mismatch: expected %v but was %v", replyNextID, replyID2)
	}
}
