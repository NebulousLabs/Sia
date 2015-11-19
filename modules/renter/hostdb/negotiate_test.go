package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

func TestNegotiateContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester("TestNegotiateContract")
	if err != nil {
		t.Fatal(err)
	}

	payout := types.NewCurrency64(1e16)

	fc := types.FileContract{
		FileSize:       0,
		FileMerkleRoot: crypto.Hash{}, // no proof possible without data
		WindowStart:    100,
		WindowEnd:      1000,
		Payout:         payout,
		ValidProofOutputs: []types.SiacoinOutput{
			{Value: types.PostTax(rt.renter.blockHeight, payout), UnlockHash: types.UnlockHash{}},
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// same as above
			{Value: types.PostTax(rt.renter.blockHeight, payout), UnlockHash: types.UnlockHash{}},
			// goes to the void, not the renter
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		UnlockHash:     types.UnlockHash{},
		RevisionNumber: 0,
	}

	txnBuilder := rt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fc.Payout)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddFileContract(fc)
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	err = rt.tpool.AcceptTransactionSet(signedTxnSet)
	if err != nil {
		t.Fatal(err)
	}

}

func TestReviseContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester("TestNegotiateContract")
	if err != nil {
		t.Fatal(err)
	}

	// get an address
	ourAddr, err := rt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}

	// generate keys
	sk, pk, err := crypto.StdKeyGen.Generate()
	if err != nil {
		t.Fatal(err)
	}
	renterPubKey := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}

	uc := types.UnlockConditions{
		PublicKeys:         []types.SiaPublicKey{renterPubKey, renterPubKey},
		SignaturesRequired: 1,
	}

	// create file contract
	payout := types.NewCurrency64(1e16)

	fc := types.FileContract{
		FileSize:       0,
		FileMerkleRoot: crypto.Hash{}, // no proof possible without data
		WindowStart:    100,
		WindowEnd:      1000,
		Payout:         payout,
		UnlockHash:     uc.UnlockHash(),
		RevisionNumber: 0,
	}
	// outputs need account for tax
	fc.ValidProofOutputs = []types.SiacoinOutput{
		{Value: types.PostTax(rt.renter.blockHeight, payout), UnlockHash: ourAddr.UnlockHash()},
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}}, // no collateral
	}
	fc.MissedProofOutputs = []types.SiacoinOutput{
		// same as above
		fc.ValidProofOutputs[0],
		// goes to the void, not the renter
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
	}

	txnBuilder := rt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fc.Payout)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddFileContract(fc)
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	// submit contract
	err = rt.tpool.AcceptTransactionSet(signedTxnSet)
	if err != nil {
		t.Fatal(err)
	}

	// create revision
	fcid := signedTxnSet[len(signedTxnSet)-1].FileContractID(0)
	rev := types.FileContractRevision{
		ParentID:              fcid,
		UnlockConditions:      uc,
		NewFileSize:           10,
		NewWindowStart:        100,
		NewWindowEnd:          1000,
		NewRevisionNumber:     1,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
	}

	// create transaction containing the revision
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(fcid),
			CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
			PublicKeyIndex: 0, // renter key is always first -- see negotiateContract
		}},
	}

	// sign the transaction
	encodedSig, err := crypto.SignHash(signedTxn.SigHash(0), sk)
	if err != nil {
		t.Fatal(err)
	}
	signedTxn.TransactionSignatures[0].Signature = encodedSig[:]

	err = signedTxn.StandaloneValid(rt.renter.blockHeight)
	if err != nil {
		t.Fatal(err)
	}

	// submit revision
	err = rt.tpool.AcceptTransactionSet([]types.Transaction{signedTxn})
	if err != nil {
		t.Fatal(err)
	}
}
