package types

import (
	"testing"
)

// TestUnlockHash runs the UnlockHash code.
func TestUnlockHash(t *testing.T) {
	uc := UnlockConditions{
		Timelock: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: SignatureEntropy,
				Key:       "fake key",
			},
		},
		NumSignatures: 3,
	}

	_ = uc.UnlockHash()
}

// TestSigHash runs the SigHash function of the transaction type.
func TestSigHash(t *testing.T) {
	txn := Transaction{
		SiacoinInputs:            []SiacoinInput{SiacoinInput{}},
		SiacoinOutputs:           []SiacoinOutput{SiacoinOutput{}},
		FileContracts:            []FileContract{FileContract{}},
		FileContractTerminations: []FileContractTermination{FileContractTermination{}},
		StorageProofs:            []StorageProof{StorageProof{}},
		SiafundInputs:            []SiafundInput{SiafundInput{}},
		SiafundOutputs:           []SiafundOutput{SiafundOutput{}},
		MinerFees:                []Currency{Currency{}},
		ArbitraryData:            []string{"one", "two"},
		Signatures: []TransactionSignature{
			TransactionSignature{
				CoveredFields: CoveredFields{
					WholeTransaction: true,
				},
			},
			TransactionSignature{
				CoveredFields: CoveredFields{
					SiacoinInputs:            []uint64{0},
					SiacoinOutputs:           []uint64{0},
					FileContracts:            []uint64{0},
					FileContractTerminations: []uint64{0},
					StorageProofs:            []uint64{0},
					SiafundInputs:            []uint64{0},
					SiafundOutputs:           []uint64{0},
					MinerFees:                []uint64{0},
					ArbitraryData:            []uint64{0},
					Signatures:               []uint64{0},
				},
			},
		},
	}

	_ = txn.SigHash(0)
	_ = txn.SigHash(1)

}

// TestSortedUnique probes the sortedUnique function.
func TestSortedUnique(t *testing.T) {
	su := []uint64{
		3,
		5,
		6,
		8,
		12,
	}
	if !sortedUnique(su, 13) {
		t.Error("sortedUnique rejected a valid array")
	}
	if sortedUnique(su, 12) {
		t.Error("sortedUnique accepted an invalid max")
	}
	if sortedUnique(su, 11) {
		t.Error("sortedUnique accepted an invalid max")
	}

	unsorted := []uint64{
		3,
		5,
		3,
	}
	if sortedUnique(unsorted, 6) {
		t.Error("sortedUnique accepted an unsorted array")
	}

	repeats := []uint64{
		2,
		4,
		4,
		7,
	}
	if sortedUnique(repeats, 8) {
		t.Error("sortedUnique accepted an array with repeats")
	}

	bothFlaws := []uint64{
		2,
		3,
		4,
		5,
		6,
		6,
		4,
	}
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
		SiacoinInputs:            []SiacoinInput{SiacoinInput{}},
		SiacoinOutputs:           []SiacoinOutput{SiacoinOutput{}},
		FileContracts:            []FileContract{FileContract{}},
		FileContractTerminations: []FileContractTermination{FileContractTermination{}},
		StorageProofs:            []StorageProof{StorageProof{}},
		SiafundInputs:            []SiafundInput{SiafundInput{}},
		SiafundOutputs:           []SiafundOutput{SiafundOutput{}},
		MinerFees:                []Currency{Currency{}},
		ArbitraryData:            []string{"one", "two"},
		Signatures: []TransactionSignature{
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
	txn.Signatures = append(txn.Signatures, TransactionSignature{
		CoveredFields: CoveredFields{
			SiacoinOutputs:           []uint64{0},
			MinerFees:                []uint64{0},
			ArbitraryData:            []uint64{0},
			FileContractTerminations: []uint64{0},
		},
	})
	err = txn.validCoveredFields()
	if err != nil {
		t.Error(err)
	}

	// Add signature coverage to the first signature. This should not violate
	// any rules.
	txn.Signatures[0].CoveredFields.Signatures = []uint64{1}
	err = txn.validCoveredFields()
	if err != nil {
		t.Error(err)
	}

	// Add siacoin output coverage to the first signature. This should violate
	// rules, as the fields are not allowed to be set when 'WholeTransaction'
	// is set.
	txn.Signatures[0].CoveredFields.SiacoinOutputs = []uint64{0}
	err = txn.validCoveredFields()
	if err != ErrWholeTransactionViolation {
		t.Error("Expecting ErrWholeTransactionViolation, got", err)
	}

	// Create a SortedUnique violation instead of a WholeTransactionViolation.
	txn.Signatures[0].CoveredFields.SiacoinOutputs = nil
	txn.Signatures[0].CoveredFields.Signatures = []uint64{1, 2}
	err = txn.validCoveredFields()
	if err != ErrSortedUniqueViolation {
		t.Error("Expecting ErrSortedUniqueViolation, got", err)
	}
}
