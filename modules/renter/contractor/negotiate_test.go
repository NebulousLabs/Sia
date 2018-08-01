package contractor

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	modWallet "github.com/NebulousLabs/Sia/modules/wallet" // name conflicts with type
	"github.com/NebulousLabs/Sia/types"
)

// contractorTester contains all of the modules that are used while testing the contractor.
type contractorTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey
	hdb       hostDB

	contractor *Contractor
}

// Close shuts down the contractor tester.
func (rt *contractorTester) Close() error {
	errs := []error{
		rt.gateway.Close(),
		rt.cs.Close(),
		rt.tpool.Close(),
		rt.miner.Close(),
		rt.wallet.Close(),
	}
	return build.JoinErrors(errs, ": ")
}

// newContractorTester creates a ready-to-use contractor tester with money in the
// wallet.
func newContractorTester(name string) (*contractorTester, error) {
	// Create the modules.
	testdir := build.TempDir("contractor", name)
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := modWallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key := crypto.GenerateTwofishKey()
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	hdb, err := hostdb.New(g, cs, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	c, err := New(cs, w, tp, hdb, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}

	// Assemble all pieces into a contractor tester.
	ct := &contractorTester{
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,
		hdb:     hdb,

		contractor: c,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := ct.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return ct, nil
}

func TestNegotiateContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ct, err := newContractorTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ct.Close()

	payout := types.NewCurrency64(1e16)

	fc := types.FileContract{
		FileSize:       0,
		FileMerkleRoot: crypto.Hash{}, // no proof possible without data
		WindowStart:    100,
		WindowEnd:      1000,
		Payout:         payout,
		ValidProofOutputs: []types.SiacoinOutput{
			{Value: types.PostTax(ct.contractor.blockHeight, payout), UnlockHash: types.UnlockHash{}},
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// same as above
			{Value: types.PostTax(ct.contractor.blockHeight, payout), UnlockHash: types.UnlockHash{}},
			// goes to the void, not the hostdb
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		UnlockHash:     types.UnlockHash{},
		RevisionNumber: 0,
	}

	txnBuilder, err := ct.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	err = txnBuilder.FundSiacoins(fc.Payout)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddFileContract(fc)
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	err = ct.tpool.AcceptTransactionSet(signedTxnSet)
	if err != nil {
		t.Fatal(err)
	}

}

func TestReviseContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ct, err := newContractorTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ct.Close()

	// get an address
	ourAddr, err := ct.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}

	// generate keys
	sk, pk := crypto.GenerateKeyPair()
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
		{Value: types.PostTax(ct.contractor.blockHeight, payout), UnlockHash: ourAddr.UnlockHash()},
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}}, // no collateral
	}
	fc.MissedProofOutputs = []types.SiacoinOutput{
		// same as above
		fc.ValidProofOutputs[0],
		// goes to the void, not the hostdb
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
	}

	txnBuilder, err := ct.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
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
	err = ct.tpool.AcceptTransactionSet(signedTxnSet)
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
			PublicKeyIndex: 0, // hostdb key is always first -- see negotiateContract
		}},
	}

	// sign the transaction
	encodedSig := crypto.SignHash(signedTxn.SigHash(0), sk)
	signedTxn.TransactionSignatures[0].Signature = encodedSig[:]

	err = signedTxn.StandaloneValid(ct.contractor.blockHeight)
	if err != nil {
		t.Fatal(err)
	}

	// submit revision
	err = ct.tpool.AcceptTransactionSet([]types.Transaction{signedTxn})
	if err != nil {
		t.Fatal(err)
	}
}
