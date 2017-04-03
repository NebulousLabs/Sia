package types

import (
	"fmt"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

func hashStr(v interface{}) string {
	h := crypto.HashObject(v)
	return fmt.Sprintf("%x", h[:])
}

// TestTransactionEncoding tests that optimizations applied to the encoding of
// the Transaction type do not change its encoding.
func TestTransactionEncoding(t *testing.T) {
	var txn Transaction
	if h := hashStr(txn); h != "143aa0da2b6a4ca39eee3ee50a6536d75eedff3b5ef0229a6d603afa7854d5b8" {
		t.Error("encoding mismatch:", h)
	}

	txn = Transaction{
		SiacoinInputs:         []SiacoinInput{{}},
		SiacoinOutputs:        []SiacoinOutput{{}},
		FileContracts:         []FileContract{{}},
		FileContractRevisions: []FileContractRevision{{}},
		StorageProofs:         []StorageProof{{}},
		SiafundInputs:         []SiafundInput{{}},
		SiafundOutputs:        []SiafundOutput{{}},
		MinerFees:             []Currency{{}},
		ArbitraryData:         [][]byte{{}},
		TransactionSignatures: []TransactionSignature{{}},
	}
	if h := hashStr(txn); h != "a6c0f41cb89aaede0682ab06c1e757e12d662a0156ec878f85b935bc219fb3ca" {
		t.Error("encoding mismatch:", h)
	}
}

// TestSiacoinInputEncoding tests that optimizations applied to the encoding
// of the SiacoinInput type do not change its encoding.
func TestSiacoinInputEncoding(t *testing.T) {
	var sci SiacoinInput
	if h := hashStr(sci); h != "2f806f905436dc7c5079ad8062467266e225d8110a3c58d17628d609cb1c99d0" {
		t.Error("encoding mismatch:", h)
	}

	sci = SiacoinInput{
		ParentID:         SiacoinOutputID{1, 2, 3},
		UnlockConditions: UnlockConditions{},
	}
	if h := hashStr(sci); h != "f172a8f5892bb2b63eff32de6fd83c132be5ad134d1227d8881632bd809ae075" {
		t.Error("encoding mismatch:", h)
	}
}

// TestSiacoinOutputEncoding tests that optimizations applied to the encoding
// of the SiacoinOutput type do not change its encoding.
func TestSiacoinOutputEncoding(t *testing.T) {
	var sco SiacoinOutput
	if h := hashStr(sco); h != "4a1931803561f431decab002e7425f0a8531d5e456a1a47fd9998a2530c0f800" {
		t.Error("encoding mismatch:", h)
	}

	sco = SiacoinOutput{
		Value:      NewCurrency64(0),
		UnlockHash: UnlockHash{1, 2, 3},
	}
	if h := hashStr(sco); h != "32fb94ae64201f3e0a373947382367666bcf205d47a58ece9260c459986ae6fd" {
		t.Error("encoding mismatch:", h)
	}
}

// TestSiafundInputEncoding tests that optimizations applied to the encoding
// of the SiafundInput type do not change its encoding.
func TestSiafundInputEncoding(t *testing.T) {
	var sci SiafundInput
	if h := hashStr(sci); h != "978a948b1a92bcddcea382bafc7718a25f8cc49b8fb11db5d9159afa960cf70a" {
		t.Error("encoding mismatch:", h)
	}

	sci = SiafundInput{
		ParentID:         SiafundOutputID{1, 2, 3},
		UnlockConditions: UnlockConditions{1, nil, 3},
		ClaimUnlockHash:  UnlockHash{1, 2, 3},
	}
	if h := hashStr(sci); h != "1a6781ca002262e1def98e294f86dd81f866e2db9029954c64a36d20d0c6b46f" {
		t.Error("encoding mismatch:", h)
	}
}

// TestSiafundOutputEncoding tests that optimizations applied to the encoding
// of the SiafundOutput type do not change its encoding.
func TestSiafundOutputEncoding(t *testing.T) {
	var sco SiafundOutput
	if h := hashStr(sco); h != "df69a516de12056d0895fdea7a0274c5aba67091543238670513104c1af69c1f" {
		t.Error("encoding mismatch:", h)
	}

	sco = SiafundOutput{
		Value:      NewCurrency64(0),
		UnlockHash: UnlockHash{1, 2, 3},
		ClaimStart: NewCurrency64(4),
	}
	if h := hashStr(sco); h != "9524d2250b21adc76967e9f86d26a68982727329e5c42a6bf5e62504891a5176" {
		t.Error("encoding mismatch:", h)
	}
}

// TestCoveredFieldsEncoding tests that optimizations applied to the encoding
// of the CoveredFields type do not change its encoding.
func TestCoveredFieldsEncoding(t *testing.T) {
	var cf CoveredFields
	if h := hashStr(cf); h != "aecfdceb8b630b5b00668d229221f876b3be1630703c4615a642db2c666b4fd7" {
		t.Error("encoding mismatch:", h)
	}

	cf = CoveredFields{
		WholeTransaction:      true,
		SiacoinInputs:         []uint64{0},
		SiacoinOutputs:        []uint64{1},
		FileContracts:         []uint64{2},
		FileContractRevisions: []uint64{3},
		StorageProofs:         []uint64{4},
		SiafundInputs:         []uint64{5},
		SiafundOutputs:        []uint64{6},
		MinerFees:             []uint64{7},
		ArbitraryData:         []uint64{8},
		TransactionSignatures: []uint64{9, 10},
	}
	if h := hashStr(cf); h != "5b10cd6b50b09447aae02829643e62b513ce99b969a80aeb620f74e77ca9bbba" {
		t.Error("encoding mismatch:", h)
	}
}

// TestSiaPublicKeyEncoding tests that optimizations applied to the encoding
// of the SiaPublicKey type do not change its encoding.
func TestSiaPublicKeyEncoding(t *testing.T) {
	var spk SiaPublicKey
	if h := hashStr(spk); h != "19ea4a516c66775ea1f648d71f6b8fa227e8b0c1a0c9203f82c33b89c4e759b5" {
		t.Error("encoding mismatch:", h)
	}

	spk = SiaPublicKey{
		Algorithm: Specifier{1, 2, 3},
		Key:       []byte{4, 5, 6},
	}
	if h := hashStr(spk); h != "9c781bbeebc23a1885d00e778c358f0a4bc81a82b48191449129752a380adc03" {
		t.Error("encoding mismatch:", h)
	}
}

// TestTransactionSignatureEncoding tests that optimizations applied to the
// encoding of the TransactionSignature type do not change its encoding.
func TestTransactionSignatureEncoding(t *testing.T) {
	var ts TransactionSignature
	if h := hashStr(ts); h != "5801097b0ae98fe7cedd4569afc11c0a433f284681ad4d66dd7181293f6d2bba" {
		t.Error("encoding mismatch:", h)
	}

	ts = TransactionSignature{
		ParentID:       crypto.Hash{1, 2, 3},
		PublicKeyIndex: 4,
		Timelock:       5,
		CoveredFields:  CoveredFields{},
		Signature:      []byte{6, 7, 8},
	}
	if h := hashStr(ts); h != "a3ce36fd8e1d6b7e5b030cdc2630d24a44472072bbd06e94d32d11132d817db0" {
		t.Error("encoding mismatch:", h)
	}
}

// TestUnlockConditionsEncoding tests that optimizations applied to the
// encoding of the UnlockConditions type do not change its encoding.
func TestUnlockConditionsEncoding(t *testing.T) {
	var uc UnlockConditions
	if h := hashStr(uc); h != "19ea4a516c66775ea1f648d71f6b8fa227e8b0c1a0c9203f82c33b89c4e759b5" {
		t.Error("encoding mismatch:", h)
	}

	uc = UnlockConditions{
		Timelock:           1,
		PublicKeys:         []SiaPublicKey{{}},
		SignaturesRequired: 3,
	}
	if h := hashStr(uc); h != "164d3741bd274d5333ab1fe8ab641b9d25cb0e0bed8e1d7bc466b5fffc956d96" {
		t.Error("encoding mismatch:", h)
	}
}
