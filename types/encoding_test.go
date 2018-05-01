package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/fastrand"
)

func hashStr(v interface{}) string {
	h := crypto.HashObject(v)
	return fmt.Sprintf("%x", h[:])
}

// heavyBlock is a complex block that fills every possible field with data.
var heavyBlock = func() Block {
	b := Block{
		MinerPayouts: []SiacoinOutput{
			{Value: CalculateCoinbase(0)},
			{Value: CalculateCoinbase(1)},
		},
		Transactions: []Transaction{
			{
				SiacoinInputs: []SiacoinInput{{
					UnlockConditions: UnlockConditions{
						PublicKeys: []SiaPublicKey{{
							Algorithm: SignatureEd25519,
							Key:       fastrand.Bytes(32),
						}},
						SignaturesRequired: 6,
					},
				}},
				SiacoinOutputs: []SiacoinOutput{{
					Value: NewCurrency64(20),
				}},
				FileContracts: []FileContract{{
					FileSize:       12,
					Payout:         NewCurrency64(100),
					RevisionNumber: 8,
					ValidProofOutputs: []SiacoinOutput{{
						Value: NewCurrency64(2),
					}},
					MissedProofOutputs: []SiacoinOutput{{
						Value: NewCurrency64(3),
					}},
				}},
				FileContractRevisions: []FileContractRevision{{
					NewFileSize:       13,
					NewRevisionNumber: 9,
					UnlockConditions: UnlockConditions{
						PublicKeys: []SiaPublicKey{{
							Algorithm: SignatureEd25519,
							Key:       fastrand.Bytes(32),
						}},
					},
					NewValidProofOutputs: []SiacoinOutput{{
						Value: NewCurrency64(4),
					}},
					NewMissedProofOutputs: []SiacoinOutput{{
						Value: NewCurrency64(5),
					}},
				}},
				StorageProofs: []StorageProof{{
					HashSet: []crypto.Hash{{}},
				}},
				SiafundInputs: []SiafundInput{{
					UnlockConditions: UnlockConditions{
						PublicKeys: []SiaPublicKey{{
							Algorithm: SignatureEd25519,
							Key:       fastrand.Bytes(32),
						}},
					},
				}},
				SiafundOutputs: []SiafundOutput{{
					ClaimStart: NewCurrency64(99),
					Value:      NewCurrency64(25),
				}},
				MinerFees:     []Currency{NewCurrency64(215)},
				ArbitraryData: [][]byte{fastrand.Bytes(10)},
				TransactionSignatures: []TransactionSignature{{
					PublicKeyIndex: 5,
					Timelock:       80,
					CoveredFields: CoveredFields{
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
						TransactionSignatures: []uint64{9},
					},
					Signature: fastrand.Bytes(32),
				}},
			},
		},
	}
	fastrand.Read(b.Transactions[0].SiacoinInputs[0].ParentID[:])
	fastrand.Read(b.Transactions[0].SiacoinOutputs[0].UnlockHash[:])
	fastrand.Read(b.Transactions[0].FileContracts[0].FileMerkleRoot[:])
	fastrand.Read(b.Transactions[0].FileContracts[0].ValidProofOutputs[0].UnlockHash[:])
	fastrand.Read(b.Transactions[0].FileContracts[0].MissedProofOutputs[0].UnlockHash[:])
	fastrand.Read(b.Transactions[0].FileContractRevisions[0].ParentID[:])
	fastrand.Read(b.Transactions[0].FileContractRevisions[0].NewFileMerkleRoot[:])
	fastrand.Read(b.Transactions[0].FileContractRevisions[0].NewValidProofOutputs[0].UnlockHash[:])
	fastrand.Read(b.Transactions[0].FileContractRevisions[0].NewMissedProofOutputs[0].UnlockHash[:])
	fastrand.Read(b.Transactions[0].StorageProofs[0].ParentID[:])
	fastrand.Read(b.Transactions[0].StorageProofs[0].HashSet[0][:])
	fastrand.Read(b.Transactions[0].StorageProofs[0].Segment[:])
	fastrand.Read(b.Transactions[0].SiafundInputs[0].ParentID[:])
	fastrand.Read(b.Transactions[0].SiafundInputs[0].ClaimUnlockHash[:])
	fastrand.Read(b.Transactions[0].SiafundOutputs[0].UnlockHash[:])
	fastrand.Read(b.Transactions[0].TransactionSignatures[0].ParentID[:])
	return b
}()

// TestBlockEncodes probes the MarshalSia and UnmarshalSia methods of the
// Block type.
func TestBlockEncoding(t *testing.T) {
	var decB Block
	err := encoding.Unmarshal(encoding.Marshal(heavyBlock), &decB)
	if err != nil {
		t.Fatal(err)
	}
	if hashStr(heavyBlock) != hashStr(decB) {
		t.Fatal("block changed after encode/decode:", heavyBlock, decB)
	}
}

// TestBadBlock tests that a known invalid encoding is not successfully
// decoded.
func TestBadBlock(t *testing.T) {
	badData := "000000000000000000000000000000000000000000000000\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	var block Block
	err := encoding.Unmarshal([]byte(badData), &block)
	if err == nil {
		t.Fatal("invalid block decoded successfully")
	}
}

// TestTooLargeDecoder tests that the decoder catches allocations that are too
// large.
func TestTooLargeDecoder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	enc := encoding.Marshal(Block{})
	// change number of transactions to large number
	copy(enc[len(enc)-8:], encoding.EncUint64(^uint64(0)))
	var block Block
	err := encoding.Unmarshal(enc, &block)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var arb [][]byte
	for i := 0; i < 4; i++ {
		arb = append(arb, make([]byte, encoding.MaxSliceSize-1))
	}
	block.Transactions = []Transaction{{
		ArbitraryData: arb,
	}}
	enc = encoding.Marshal(block)
	err = encoding.Unmarshal(enc, &block)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestCurrencyMarshalJSON probes the MarshalJSON and UnmarshalJSON functions
// of the currency type.
func TestCurrencyMarshalJSON(t *testing.T) {
	b30 := big.NewInt(30)
	c30 := NewCurrency64(30)

	bMar30, err := b30.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	cMar30, err := c30.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bMar30, bytes.Trim(cMar30, `"`)) {
		t.Error("Currency does not match the marshalling of its math/big equivalent")
	}

	var cUmar30 Currency
	err = cUmar30.UnmarshalJSON(cMar30)
	if err != nil {
		t.Fatal(err)
	}
	if c30.Cmp(cUmar30) != 0 {
		t.Error("Incorrect unmarshalling of currency type.")
	}

	cMar30[0] = 0
	err = cUmar30.UnmarshalJSON(cMar30)
	if err == nil {
		t.Error("JSON decoded nonsense input")
	}
}

// TestCurrencyMarshalSia probes the MarshalSia and UnmarshalSia functions of
// the currency type.
func TestCurrencyMarshalSia(t *testing.T) {
	c := NewCurrency64(1656)
	buf := new(bytes.Buffer)
	err := c.MarshalSia(buf)
	if err != nil {
		t.Fatal(err)
	}
	var cUmar Currency
	cUmar.UnmarshalSia(buf)
	if c.Cmp(cUmar) != 0 {
		t.Error("marshal and unmarshal mismatch for currency type")
	}
}

// TestCurrencyString probes the String function of the currency type.
func TestCurrencyString(t *testing.T) {
	b := big.NewInt(7135)
	c := NewCurrency64(7135)
	if b.String() != c.String() {
		t.Error("string function not behaving as expected")
	}
}

// TestCurrencyScan probes the Scan function of the currency type.
func TestCurrencyScan(t *testing.T) {
	var c0 Currency
	c1 := NewCurrency64(81293)
	_, err := fmt.Sscan("81293", &c0)
	if err != nil {
		t.Fatal(err)
	}
	if c0.Cmp(c1) != 0 {
		t.Error("scanned number does not equal expected value")
	}
	_, err = fmt.Sscan("z", &c0)
	if err == nil {
		t.Fatal("scan is accepting garbage input")
	}
}

// TestCurrencyEncoding checks that a currency can encode and decode without
// error.
func TestCurrencyEncoding(t *testing.T) {
	c := NewCurrency64(351)
	cMar := encoding.Marshal(c)
	var cUmar Currency
	err := encoding.Unmarshal(cMar, &cUmar)
	if err != nil {
		t.Error("Error unmarshalling a currency:", err)
	}
	if cUmar.Cmp(c) != 0 {
		t.Error("Marshalling and Unmarshalling a currency did not work correctly")
	}
}

// TestNegativeCurrencyUnmarshalJSON tries to unmarshal a negative number from
// JSON.
func TestNegativeCurrencyUnmarshalJSON(t *testing.T) {
	// Marshal a 2 digit number.
	c := NewCurrency64(35)
	cMar, err := c.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	// Change the first digit to a negative character.
	cMar[0] = 45

	// Try unmarshalling the negative currency.
	var cNeg Currency
	err = cNeg.UnmarshalJSON(cMar)
	if err != ErrNegativeCurrency {
		t.Error("expecting ErrNegativeCurrency:", err)
	}
	if cNeg.i.Sign() < 0 {
		t.Error("negative currency returned")
	}
}

// TestNegativeCurrencyScan tries to scan in a negative number and checks for
// an error.
func TestNegativeCurrencyScan(t *testing.T) {
	var c Currency
	_, err := fmt.Sscan("-23", &c)
	if err != ErrNegativeCurrency {
		t.Error("expecting ErrNegativeCurrency:", err)
	}
}

// TestCurrencyUnsafeDecode tests that decoding into an existing Currency
// value does not overwrite its contents.
func TestCurrencyUnsafeDecode(t *testing.T) {
	// Scan
	backup := SiacoinPrecision.Mul64(1)
	c := SiacoinPrecision
	_, err := fmt.Sscan("7", &c)
	if err != nil {
		t.Error(err)
	} else if !SiacoinPrecision.Equals(backup) {
		t.Errorf("Scan changed value of SiacoinPrecision: %v -> %v", backup, SiacoinPrecision)
	}

	// UnmarshalSia
	c = SiacoinPrecision
	err = encoding.Unmarshal(encoding.Marshal(NewCurrency64(7)), &c)
	if err != nil {
		t.Error(err)
	} else if !SiacoinPrecision.Equals(backup) {
		t.Errorf("UnmarshalSia changed value of SiacoinPrecision: %v -> %v", backup, SiacoinPrecision)
	}
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

// TestSiaPublicKeyLoadString checks that the LoadString method is the proper
// inverse of the String() method, also checks that there are no stupid panics
// or severe errors.
func TestSiaPublicKeyLoadString(t *testing.T) {
	spk := SiaPublicKey{
		Algorithm: SignatureEd25519,
		Key:       fastrand.Bytes(32),
	}

	spkString := spk.String()
	var loadedSPK SiaPublicKey
	loadedSPK.LoadString(spkString)
	if !bytes.Equal(loadedSPK.Algorithm[:], spk.Algorithm[:]) {
		t.Error("SiaPublicKey is not loading correctly")
	}
	if !bytes.Equal(loadedSPK.Key, spk.Key) {
		t.Log(loadedSPK.Key, spk.Key)
		t.Error("SiaPublicKey is not loading correctly")
	}

	// Try loading crappy strings.
	parts := strings.Split(spkString, ":")
	spk.LoadString(parts[0])
	spk.LoadString(parts[0][1:])
	spk.LoadString(parts[0][:1])
	spk.LoadString(parts[1])
	spk.LoadString(parts[1][1:])
	spk.LoadString(parts[1][:1])
	spk.LoadString(parts[0] + parts[1])

}

// TestSiaPublicKeyString does a quick check to verify that the String method
// on the SiaPublicKey is producing the expected output.
func TestSiaPublicKeyString(t *testing.T) {
	spk := SiaPublicKey{
		Algorithm: SignatureEd25519,
		Key:       make([]byte, 32),
	}

	if spk.String() != "ed25519:0000000000000000000000000000000000000000000000000000000000000000" {
		t.Error("got wrong value for spk.String():", spk.String())
	}
}

// TestSpecifierMarshaling tests the marshaling methods of the specifier
// type.
func TestSpecifierMarshaling(t *testing.T) {
	s1 := SpecifierClaimOutput
	b, err := json.Marshal(s1)
	if err != nil {
		t.Fatal(err)
	}
	var s2 Specifier
	err = json.Unmarshal(b, &s2)
	if err != nil {
		t.Fatal(err)
	} else if s2 != s1 {
		t.Fatal("mismatch:", s1, s2)
	}

	// invalid json
	x := 3
	b, _ = json.Marshal(x)
	err = json.Unmarshal(b, &s2)
	if err == nil {
		t.Fatal("Unmarshal should have failed")
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

// TestUnlockHashJSONMarshalling checks that when an unlock hash is marshalled
// and unmarshalled using JSON, the result is what is expected.
func TestUnlockHashJSONMarshalling(t *testing.T) {
	// Create an unlock hash.
	uc := UnlockConditions{
		Timelock:           5,
		SignaturesRequired: 3,
	}
	uh := uc.UnlockHash()

	// Marshal the unlock hash.
	marUH, err := json.Marshal(uh)
	if err != nil {
		t.Fatal(err)
	}

	// Unmarshal the unlock hash and compare to the original.
	var umarUH UnlockHash
	err = json.Unmarshal(marUH, &umarUH)
	if err != nil {
		t.Fatal(err)
	}
	if umarUH != uh {
		t.Error("Marshalled and unmarshalled unlock hash are not equivalent")
	}

	// Corrupt the checksum.
	marUH[36]++
	err = umarUH.UnmarshalJSON(marUH)
	if err != ErrInvalidUnlockHashChecksum {
		t.Error("expecting an invalid checksum:", err)
	}
	marUH[36]--

	// Try an input that's not correct hex.
	marUH[7] += 100
	err = umarUH.UnmarshalJSON(marUH)
	if err == nil {
		t.Error("Expecting error after corrupting input")
	}
	marUH[7] -= 100

	// Try an input of the wrong length.
	err = (&umarUH).UnmarshalJSON(marUH[2:])
	if err != ErrUnlockHashWrongLen {
		t.Error("Got wrong error:", err)
	}
}

// TestUnlockHashStringMarshalling checks that when an unlock hash is
// marshalled and unmarshalled using String and LoadString, the result is what
// is expected.
func TestUnlockHashStringMarshalling(t *testing.T) {
	// Create an unlock hash.
	uc := UnlockConditions{
		Timelock:           2,
		SignaturesRequired: 7,
	}
	uh := uc.UnlockHash()

	// Marshal the unlock hash.
	marUH := uh.String()

	// Unmarshal the unlock hash and compare to the original.
	var umarUH UnlockHash
	err := umarUH.LoadString(marUH)
	if err != nil {
		t.Fatal(err)
	}
	if umarUH != uh {
		t.Error("Marshalled and unmarshalled unlock hash are not equivalent")
	}

	// Corrupt the checksum.
	byteMarUH := []byte(marUH)
	byteMarUH[36]++
	err = umarUH.LoadString(string(byteMarUH))
	if err != ErrInvalidUnlockHashChecksum {
		t.Error("expecting an invalid checksum:", err)
	}
	byteMarUH[36]--

	// Try an input that's not correct hex.
	byteMarUH[7] += 100
	err = umarUH.LoadString(string(byteMarUH))
	if err == nil {
		t.Error("Expecting error after corrupting input")
	}
	byteMarUH[7] -= 100

	// Try an input of the wrong length.
	err = umarUH.LoadString(string(byteMarUH[2:]))
	if err != ErrUnlockHashWrongLen {
		t.Error("Got wrong error:", err)
	}
}

// TestCurrencyHumanString checks that the HumanString method of the currency
// type is correctly formatting values.
func TestCurrencyUnits(t *testing.T) {
	tests := []struct {
		in  Currency
		out string
	}{
		{NewCurrency64(1), "1 H"},
		{NewCurrency64(1000), "1000 H"},
		{NewCurrency64(100000000000), "100000000000 H"},
		{NewCurrency64(1000000000000), "1 pS"},
		{NewCurrency64(1234560000000), "1.235 pS"},
		{NewCurrency64(12345600000000), "12.35 pS"},
		{NewCurrency64(123456000000000), "123.5 pS"},
		{NewCurrency64(1000000000000000), "1 nS"},
		{NewCurrency64(1000000000000000000), "1 uS"},
		{NewCurrency64(1000000000).Mul64(1000000000000), "1 mS"},
		{NewCurrency64(1).Mul(SiacoinPrecision), "1 SC"},
		{NewCurrency64(1000).Mul(SiacoinPrecision), "1 KS"},
		{NewCurrency64(1000000).Mul(SiacoinPrecision), "1 MS"},
		{NewCurrency64(1000000000).Mul(SiacoinPrecision), "1 GS"},
		{NewCurrency64(1000000000000).Mul(SiacoinPrecision), "1 TS"},
		{NewCurrency64(1234560000000).Mul(SiacoinPrecision), "1.235 TS"},
		{NewCurrency64(1234560000000000).Mul(SiacoinPrecision), "1235 TS"},
	}
	for _, test := range tests {
		if test.in.HumanString() != test.out {
			t.Errorf("currencyUnits(%v): expected %v, got %v", test.in, test.out, test.in.HumanString())
		}
	}
}

// TestTransactionMarshalSiaSize tests that the txn.MarshalSiaSize method is
// always consistent with len(encoding.Marshal(txn)).
func TestTransactionMarshalSiaSize(t *testing.T) {
	txn := Transaction{
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
	if txn.MarshalSiaSize() != len(encoding.Marshal(txn)) {
		t.Errorf("sizes do not match: expected %v, got %v", len(encoding.Marshal(txn)), txn.MarshalSiaSize())
	}
}

// TestUnlockHashScan checks if the fmt.Scanner implementation of UnlockHash
// works as expected.
func TestUnlockHashScan(t *testing.T) {
	// Create a random unlock hash.
	var uh UnlockHash
	fastrand.Read(uh[:])
	// Convert it to a string and parse the string using Sscan.
	var scannedHash UnlockHash
	fmt.Sscan(uh.String(), &scannedHash)
	// Check if they are equal.
	if !bytes.Equal(uh[:], scannedHash[:]) {
		t.Fatal("scanned hash is not equal to original hash")
	}
}
