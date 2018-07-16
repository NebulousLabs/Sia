package api

import (
	"encoding/json"
	"testing"

	"gitlab.com/NebulousLabs/Sia/types"
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

	// Get a transaction to validate.
	txnSet, err := st.wallet.SendSiacoins(types.SiacoinPrecision, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}

	jsonTxns, err := json.Marshal(txnSet)
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

	// Try again with an invalid transaction set
	txnSet = []types.Transaction{{TransactionSignatures: []types.TransactionSignature{{}}}}
	jsonTxns, err = json.Marshal(txnSet)
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
