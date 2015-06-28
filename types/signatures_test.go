package types

import (
	"encoding/json"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// TestUnlockHash runs the UnlockHash code.
func TestUnlockHash(t *testing.T) {
	uc := UnlockConditions{
		Timelock: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: SignatureEntropy,
				Key:       []byte{'f', 'a', 'k', 'e'},
			},
		},
		SignaturesRequired: 3,
	}

	_ = uc.UnlockHash()
}

// TestSigHash runs the SigHash function of the transaction type.
func TestSigHash(t *testing.T) {
	txn := Transaction{
		SiacoinInputs:         []SiacoinInput{SiacoinInput{}},
		SiacoinOutputs:        []SiacoinOutput{SiacoinOutput{}},
		FileContracts:         []FileContract{FileContract{}},
		FileContractRevisions: []FileContractRevision{FileContractRevision{}},
		StorageProofs:         []StorageProof{StorageProof{}},
		SiafundInputs:         []SiafundInput{SiafundInput{}},
		SiafundOutputs:        []SiafundOutput{SiafundOutput{}},
		MinerFees:             []Currency{Currency{}},
		ArbitraryData:         [][]byte{[]byte{'o'}, []byte{'t'}},
		TransactionSignatures: []TransactionSignature{
			TransactionSignature{
				CoveredFields: CoveredFields{
					WholeTransaction: true,
				},
			},
			TransactionSignature{
				CoveredFields: CoveredFields{
					SiacoinInputs:         []uint64{0},
					SiacoinOutputs:        []uint64{0},
					FileContracts:         []uint64{0},
					FileContractRevisions: []uint64{0},
					StorageProofs:         []uint64{0},
					SiafundInputs:         []uint64{0},
					SiafundOutputs:        []uint64{0},
					MinerFees:             []uint64{0},
					ArbitraryData:         []uint64{0},
					TransactionSignatures: []uint64{0},
				},
			},
		},
	}

	_ = txn.SigHash(0)
	_ = txn.SigHash(1)

}

// TestSortedUnique probes the sortedUnique function.
func TestSortedUnique(t *testing.T) {
	su := []uint64{3, 5, 6, 8, 12}
	if !sortedUnique(su, 13) {
		t.Error("sortedUnique rejected a valid array")
	}
	if sortedUnique(su, 12) {
		t.Error("sortedUnique accepted an invalid max")
	}
	if sortedUnique(su, 11) {
		t.Error("sortedUnique accepted an invalid max")
	}

	unsorted := []uint64{3, 5, 3}
	if sortedUnique(unsorted, 6) {
		t.Error("sortedUnique accepted an unsorted array")
	}

	repeats := []uint64{2, 4, 4, 7}
	if sortedUnique(repeats, 8) {
		t.Error("sortedUnique accepted an array with repeats")
	}

	bothFlaws := []uint64{2, 3, 4, 5, 6, 6, 4}
	if sortedUnique(bothFlaws, 7) {
		t.Error("Sorted unique accetped array with multiple flaws")
	}
}

// TestTransactionValidCoveredFields probes the validCoveredFields menthod of
// the transaction type.
func TestTransactionValidCoveredFields(t *testing.T) {
	// Create a transaction with all fields filled in minimally. The first
	// check has a legal CoveredFields object with 'WholeTransaction' set.
	txn := Transaction{
		SiacoinInputs:         []SiacoinInput{SiacoinInput{}},
		SiacoinOutputs:        []SiacoinOutput{SiacoinOutput{}},
		FileContracts:         []FileContract{FileContract{}},
		FileContractRevisions: []FileContractRevision{FileContractRevision{}},
		StorageProofs:         []StorageProof{StorageProof{}},
		SiafundInputs:         []SiafundInput{SiafundInput{}},
		SiafundOutputs:        []SiafundOutput{SiafundOutput{}},
		MinerFees:             []Currency{Currency{}},
		ArbitraryData:         [][]byte{[]byte{'o'}, []byte{'t'}},
		TransactionSignatures: []TransactionSignature{
			TransactionSignature{
				CoveredFields: CoveredFields{
					WholeTransaction: true,
				},
			},
		},
	}
	err := txn.validCoveredFields()
	if err != nil {
		t.Error(err)
	}

	// Second check has CoveredFields object where 'WholeTransaction' is not
	// set.
	txn.TransactionSignatures = append(txn.TransactionSignatures, TransactionSignature{
		CoveredFields: CoveredFields{
			SiacoinOutputs:        []uint64{0},
			MinerFees:             []uint64{0},
			ArbitraryData:         []uint64{0},
			FileContractRevisions: []uint64{0},
		},
	})
	err = txn.validCoveredFields()
	if err != nil {
		t.Error(err)
	}

	// Add signature coverage to the first signature. This should not violate
	// any rules.
	txn.TransactionSignatures[0].CoveredFields.TransactionSignatures = []uint64{1}
	err = txn.validCoveredFields()
	if err != nil {
		t.Error(err)
	}

	// Add siacoin output coverage to the first signature. This should violate
	// rules, as the fields are not allowed to be set when 'WholeTransaction'
	// is set.
	txn.TransactionSignatures[0].CoveredFields.SiacoinOutputs = []uint64{0}
	err = txn.validCoveredFields()
	if err != ErrWholeTransactionViolation {
		t.Error("Expecting ErrWholeTransactionViolation, got", err)
	}

	// Create a SortedUnique violation instead of a WholeTransactionViolation.
	txn.TransactionSignatures[0].CoveredFields.SiacoinOutputs = nil
	txn.TransactionSignatures[0].CoveredFields.TransactionSignatures = []uint64{1, 2}
	err = txn.validCoveredFields()
	if err != ErrSortedUniqueViolation {
		t.Error("Expecting ErrSortedUniqueViolation, got", err)
	}
}

// TestTransactionValidSignatures probes the validSignatures method of the
// Transaction type.
func TestTransactionValidSignatures(t *testing.T) {
	// Create keys for use in signing and verifying.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		t.Fatal(err)
	}

	// Create UnlockConditions with 3 keys, 2 of which are required. The first
	// possible key is a standard signature. The second key is an unknown
	// signature type, which should always be accepted. The final type is an
	// entropy type, which should never be accepted.
	uc := UnlockConditions{
		PublicKeys: []SiaPublicKey{
			{Algorithm: SignatureEd25519, Key: pk[:]},
			{},
			{Algorithm: SignatureEntropy},
		},
		SignaturesRequired: 2,
	}

	// Create a transaction with each type of unlock condition.
	txn := Transaction{
		SiacoinInputs: []SiacoinInput{
			{UnlockConditions: uc},
		},
		FileContractRevisions: []FileContractRevision{
			{UnlockConditions: uc},
		},
		SiafundInputs: []SiafundInput{
			{UnlockConditions: uc},
		},
	}
	txn.FileContractRevisions[0].ParentID[0] = 1 // can't overlap with other objects
	txn.SiafundInputs[0].ParentID[0] = 2         // can't overlap with other objects

	// Create the signatures that spend the output.
	txn.TransactionSignatures = []TransactionSignature{
		// First signatures use cryptography.
		{
			Timelock:      5,
			CoveredFields: CoveredFields{WholeTransaction: true},
		},
		{
			CoveredFields: CoveredFields{WholeTransaction: true},
		},
		{
			CoveredFields: CoveredFields{WholeTransaction: true},
		},

		// The second signatures should always work for being unrecognized
		// types.
		{PublicKeyIndex: 1},
		{PublicKeyIndex: 1},
		{PublicKeyIndex: 1},
	}
	txn.TransactionSignatures[1].ParentID[0] = 1
	txn.TransactionSignatures[2].ParentID[0] = 2
	txn.TransactionSignatures[4].ParentID[0] = 1
	txn.TransactionSignatures[5].ParentID[0] = 2
	sigHash0 := txn.SigHash(0)
	sigHash1 := txn.SigHash(1)
	sigHash2 := txn.SigHash(2)
	sig0, err := crypto.SignHash(sigHash0, sk)
	if err != nil {
		t.Fatal(err)
	}
	sig1, err := crypto.SignHash(sigHash1, sk)
	if err != nil {
		t.Fatal(err)
	}
	sig2, err := crypto.SignHash(sigHash2, sk)
	if err != nil {
		t.Fatal(err)
	}
	txn.TransactionSignatures[0].Signature = sig0[:]
	txn.TransactionSignatures[1].Signature = sig1[:]
	txn.TransactionSignatures[2].Signature = sig2[:]

	// Check that the signing was successful.
	err = txn.validSignatures(10)
	if err != nil {
		t.Error(err)
	}

	// Corrupt one of the sigantures.
	sig0[0]++
	txn.TransactionSignatures[0].Signature = sig0[:]
	err = txn.validSignatures(10)
	if err == nil {
		t.Error("Corrupted a signature but the txn was still accepted as valid!")
	}
	sig0[0]--
	txn.TransactionSignatures[0].Signature = sig0[:]

	// Fail the validCoveredFields check.
	txn.TransactionSignatures[0].CoveredFields.SiacoinInputs = []uint64{33}
	err = txn.validSignatures(10)
	if err == nil {
		t.Error("failed to flunk the validCoveredFields check")
	}
	txn.TransactionSignatures[0].CoveredFields.SiacoinInputs = nil

	// Double spend a SiacoinInput, FileContractTermination, and SiafundInput.
	txn.SiacoinInputs = append(txn.SiacoinInputs, SiacoinInput{UnlockConditions: UnlockConditions{}})
	err = txn.validSignatures(10)
	if err == nil {
		t.Error("failed to double spend a siacoin input")
	}
	txn.SiacoinInputs = txn.SiacoinInputs[:len(txn.SiacoinInputs)-1]
	txn.FileContractRevisions = append(txn.FileContractRevisions, FileContractRevision{UnlockConditions: UnlockConditions{}})
	err = txn.validSignatures(10)
	if err == nil {
		t.Error("failed to double spend a file contract termination")
	}
	txn.FileContractRevisions = txn.FileContractRevisions[:len(txn.FileContractRevisions)-1]
	txn.SiafundInputs = append(txn.SiafundInputs, SiafundInput{UnlockConditions: UnlockConditions{}})
	err = txn.validSignatures(10)
	if err == nil {
		t.Error("failed to double spend a siafund input")
	}
	txn.SiafundInputs = txn.SiafundInputs[:len(txn.SiafundInputs)-1]

	// Add a frivilous signature
	txn.TransactionSignatures = append(txn.TransactionSignatures, TransactionSignature{})
	err = txn.validSignatures(10)
	if err != ErrFrivilousSignature {
		t.Error(err)
	}
	txn.TransactionSignatures = txn.TransactionSignatures[:len(txn.TransactionSignatures)-1]

	// Replace one of the cryptography signatures with an always-accepted
	// signature. This should get rejected because the always-accepted
	// signature has already been used.
	tmpTxn0 := txn.TransactionSignatures[0]
	txn.TransactionSignatures[0] = TransactionSignature{PublicKeyIndex: 1}
	err = txn.validSignatures(10)
	if err != ErrPublicKeyOveruse {
		t.Error(err)
	}
	txn.TransactionSignatures[0] = tmpTxn0

	// Fail the timelock check for signatures.
	err = txn.validSignatures(4)
	if err != ErrPrematureSignature {
		t.Error(err)
	}

	// Try to spend an entropy signature.
	txn.TransactionSignatures[0] = TransactionSignature{PublicKeyIndex: 2}
	err = txn.validSignatures(10)
	if err != ErrEntropyKey {
		t.Error(err)
	}
	txn.TransactionSignatures[0] = tmpTxn0

	// Try to point to a nonexistant public key.
	txn.TransactionSignatures[0] = TransactionSignature{PublicKeyIndex: 5}
	err = txn.validSignatures(10)
	if err != ErrInvalidPubKeyIndex {
		t.Error(err)
	}
	txn.TransactionSignatures[0] = tmpTxn0

	// Insert a malformed public key into the transaction.
	txn.SiacoinInputs[0].UnlockConditions.PublicKeys[0].Key = []byte{'b', 'a', 'd'}
	err = txn.validSignatures(10)
	if err == nil {
		t.Error(err)
	}
	txn.SiacoinInputs[0].UnlockConditions.PublicKeys[0].Key = pk[:]

	// Insert a malformed signature into the transaction.
	txn.TransactionSignatures[0].Signature = []byte{'m', 'a', 'l'}
	err = txn.validSignatures(10)
	if err == nil {
		t.Error(err)
	}
	txn.TransactionSignatures[0] = tmpTxn0

	// Try to spend a transaction when not every required signature is
	// available.
	txn.TransactionSignatures = txn.TransactionSignatures[1:]
	err = txn.validSignatures(10)
	if err != ErrMissingSignatures {
		t.Error(err)
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
