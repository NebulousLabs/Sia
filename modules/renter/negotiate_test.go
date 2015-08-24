package renter

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
			{Value: payout, UnlockHash: types.UnlockHash{}},
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// same as above
			{Value: payout, UnlockHash: types.UnlockHash{}},
			// goes to the void, not the renter
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		UnlockHash:     types.UnlockHash{},
		RevisionNumber: 0,
	}
	fc.ValidProofOutputs[0].Value = fc.ValidProofOutputs[0].Value.Sub(fc.Tax())
	fc.MissedProofOutputs[0].Value = fc.MissedProofOutputs[0].Value.Sub(fc.Tax())

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

	// generate keys
	sk, pk, err := crypto.GenerateSignatureKeys()
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

	// create revision
	fcid := types.FileContractID{1}
	rev := types.FileContractRevision{
		ParentID:         fcid,
		UnlockConditions: uc,
		NewFileSize:      10,
		NewWindowStart:   100,
		NewWindowEnd:     1000,
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
}
