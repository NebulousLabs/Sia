package hostdb

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	modWallet "github.com/NebulousLabs/Sia/modules/wallet" // name conflicts with type
	"github.com/NebulousLabs/Sia/types"
)

// hostdbTester contains all of the modules that are used while testing the hostdb.
type hostdbTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	hostdb *HostDB
}

// Close shuts down the hostdb tester.
func (rt *hostdbTester) Close() error {
	rt.wallet.Lock()
	rt.cs.Close()
	rt.gateway.Close()
	return nil
}

// newHostDBTester creates a ready-to-use hostdb tester with money in the
// wallet.
func newHostDBTester(name string) (*hostdbTester, error) {
	// Create the modules.
	testdir := build.TempDir("hostdb", name)
	g, err := gateway.New("localhost:0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, err
	}
	w, err := modWallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	hdb, err := New(cs, w, tp, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all pieces into a hostdb tester.
	ht := &hostdbTester{
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		hostdb: hdb,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return ht, nil
}

func TestNegotiateContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostDBTester("TestNegotiateContract")
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
			{Value: types.PostTax(ht.hostdb.blockHeight, payout), UnlockHash: types.UnlockHash{}},
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// same as above
			{Value: types.PostTax(ht.hostdb.blockHeight, payout), UnlockHash: types.UnlockHash{}},
			// goes to the void, not the hostdb
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		UnlockHash:     types.UnlockHash{},
		RevisionNumber: 0,
	}

	txnBuilder := ht.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fc.Payout)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddFileContract(fc)
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	err = ht.tpool.AcceptTransactionSet(signedTxnSet)
	if err != nil {
		t.Fatal(err)
	}

}

func TestReviseContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostDBTester("TestNegotiateContract")
	if err != nil {
		t.Fatal(err)
	}

	// get an address
	ourAddr, err := ht.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}

	// generate keys
	sk, pk, err := crypto.GenerateKeyPair()
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
		{Value: types.PostTax(ht.hostdb.blockHeight, payout), UnlockHash: ourAddr.UnlockHash()},
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}}, // no collateral
	}
	fc.MissedProofOutputs = []types.SiacoinOutput{
		// same as above
		fc.ValidProofOutputs[0],
		// goes to the void, not the hostdb
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
	}

	txnBuilder := ht.wallet.StartTransaction()
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
	err = ht.tpool.AcceptTransactionSet(signedTxnSet)
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
	encodedSig, err := crypto.SignHash(signedTxn.SigHash(0), sk)
	if err != nil {
		t.Fatal(err)
	}
	signedTxn.TransactionSignatures[0].Signature = encodedSig[:]

	err = signedTxn.StandaloneValid(ht.hostdb.blockHeight)
	if err != nil {
		t.Fatal(err)
	}

	// submit revision
	err = ht.tpool.AcceptTransactionSet([]types.Transaction{signedTxn})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUniqueHosts tests the UniqueHosts method of the pool type.
// TODO: make pool safer to test, using real or mocked hostdb
func TestUniqueHosts(t *testing.T) {
	// create hosts
	h1, h2, h3 := new(hostUploader), new(hostUploader), new(hostUploader)
	h1.contract.IP = fakeAddr(1)
	h2.contract.IP = fakeAddr(2)
	h3.contract.IP = fakeAddr(3)

	p := &pool{hosts: []*hostUploader{h1, h2, h3}}

	tests := []struct {
		n      int
		hosts  []modules.NetAddress
		expLen int
	}{
		{0, nil, 0},
		{3, nil, 3},
		{2, []modules.NetAddress{fakeAddr(1)}, 2},
		// TODO: these tests cause panics
		//{4, nil, 3},
		//{1, []modules.NetAddress{fakeAddr(1), fakeAddr(2), fakeAddr(3)}, 0},
	}
	for i, test := range tests {
		hosts := p.UniqueHosts(test.n, test.hosts)
		if len(hosts) != test.expLen {
			t.Errorf("test %v failed: expected %v hosts, got %v", i, test.expLen, len(hosts))
		}
	}
}
