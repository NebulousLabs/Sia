package types

import (
	"testing"
)

// TestTransactionCorrectFileContracts probes the correctFileContracts function
// of the Transaction type.
func TestTransactionCorrectFileContracts(t *testing.T) {
	// Try a transaction with a FileContract that is correct.
	txn := Transaction{
		FileContracts: []FileContract{
			FileContract{
				WindowStart: 35,
				WindowEnd:   40,
				Payout:      NewCurrency64(1e6),
				ValidProofOutputs: []SiacoinOutput{
					SiacoinOutput{
						Value: NewCurrency64(70e3),
					},
					SiacoinOutput{
						Value: NewCurrency64(900e3),
					},
				},
				MissedProofOutputs: []SiacoinOutput{
					SiacoinOutput{
						Value: NewCurrency64(100e3),
					},
					SiacoinOutput{
						Value: NewCurrency64(900e3),
					},
				},
			},
		},
	}
	err := txn.correctFileContracts(30)
	if err != nil {
		t.Error(err)
	}

	// Try when the start height was missed.
	err = txn.correctFileContracts(35)
	if err != ErrFileContractWindowStartViolation {
		t.Error(err)
	}
	err = txn.correctFileContracts(135)
	if err != ErrFileContractWindowStartViolation {
		t.Error(err)
	}

	// Try when the expiration equal to and less than the start.
	txn.FileContracts[0].WindowEnd = 35
	err = txn.correctFileContracts(30)
	if err != ErrFileContractExpirationViolation {
		t.Error(err)
	}
	txn.FileContracts[0].WindowEnd = 35
	err = txn.correctFileContracts(30)
	if err != ErrFileContractExpirationViolation {
		t.Error(err)
	}
	txn.FileContracts[0].WindowEnd = 40

	// Attempt under and over output sums.
	txn.FileContracts[0].ValidProofOutputs[0].Value = NewCurrency64(69e3)
	err = txn.correctFileContracts(30)
	if err != ErrFileContractOutputSumViolation {
		t.Error(err)
	}
	txn.FileContracts[0].ValidProofOutputs[0].Value = NewCurrency64(71e3)
	err = txn.correctFileContracts(30)
	if err != ErrFileContractOutputSumViolation {
		t.Error(err)
	}
	txn.FileContracts[0].ValidProofOutputs[0].Value = NewCurrency64(70e3)

	txn.FileContracts[0].MissedProofOutputs[0].Value = NewCurrency64(99e3)
	err = txn.correctFileContracts(30)
	if err != ErrFileContractOutputSumViolation {
		t.Error(err)
	}
	txn.FileContracts[0].MissedProofOutputs[0].Value = NewCurrency64(101e3)
	err = txn.correctFileContracts(30)
	if err != ErrFileContractOutputSumViolation {
		t.Error(err)
	}
	txn.FileContracts[0].MissedProofOutputs[0].Value = NewCurrency64(100e3)

	// Try the payouts when the value of the contract is too low to incur a
	// fee.
	txn.FileContracts = append(txn.FileContracts, FileContract{
		WindowStart: 35,
		WindowEnd:   40,
		Payout:      NewCurrency64(1e3),
		ValidProofOutputs: []SiacoinOutput{
			SiacoinOutput{
				Value: NewCurrency64(1e3),
			},
		},
		MissedProofOutputs: []SiacoinOutput{
			SiacoinOutput{
				Value: NewCurrency64(1e3),
			},
		},
	})
	err = txn.correctFileContracts(30)
	if err != nil {
		t.Error(err)
	}
}

// TestTransactionFitsInABlock probes the fitsInABlock method of the
// Transaction type.
func TestTransactionFitsInABlock(t *testing.T) {
	// Try a transaction that will fit in a block, followed by one that won't.
	data := make([]byte, BlockSizeLimit/2)
	txn := Transaction{ArbitraryData: []string{string(data)}}
	err := txn.fitsInABlock()
	if err != nil {
		t.Error(err)
	}
	data = make([]byte, BlockSizeLimit)
	txn.ArbitraryData[0] = string(data)
	err = txn.fitsInABlock()
	if err != ErrTransactionTooLarge {
		t.Error(err)
	}
}

// TestTransactionFollowsMinimumValues probes the followsMinimumValues method
// of the Transaction type.
func TestTransactionFollowsMinimumValues(t *testing.T) {
	// Start with a transaction that follows all of minimum-values rules.
	txn := Transaction{
		SiacoinOutputs: []SiacoinOutput{SiacoinOutput{Value: NewCurrency64(1)}},
		FileContracts:  []FileContract{FileContract{Payout: NewCurrency64(1)}},
		SiafundOutputs: []SiafundOutput{SiafundOutput{Value: NewCurrency64(1)}},
	}
	err := txn.followsMinimumValues()
	if err != nil {
		t.Error(err)
	}

	// Try a zero value for each type.
	txn.SiacoinOutputs[0].Value = ZeroCurrency
	err = txn.followsMinimumValues()
	if err != ErrZeroOutput {
		t.Error(err)
	}
	txn.SiacoinOutputs[0].Value = NewCurrency64(1)
	txn.FileContracts[0].Payout = ZeroCurrency
	err = txn.followsMinimumValues()
	if err != ErrZeroOutput {
		t.Error(err)
	}
	txn.FileContracts[0].Payout = NewCurrency64(1)
	txn.SiafundOutputs[0].Value = ZeroCurrency
	err = txn.followsMinimumValues()
	if err != ErrZeroOutput {
		t.Error(err)
	}
	txn.SiafundOutputs[0].Value = NewCurrency64(1)

	// Try a non-zero value for the ClaimStart field of a siafund output.
	txn.SiafundOutputs[0].ClaimStart = NewCurrency64(1)
	err = txn.followsMinimumValues()
	if err != ErrNonZeroClaimStart {
		t.Error(err)
	}
	txn.SiafundOutputs[0].ClaimStart = ZeroCurrency
}

// TestTransactionFollowsStorageProofRules probes the followsStorageProofRules
// method of the Transaction type.
func TestTransactionFollowsStorageProofRules(t *testing.T) {
	// Try a transaction with no storage proofs.
	txn := Transaction{}
	err := txn.followsStorageProofRules()
	if err != nil {
		t.Error(err)
	}

	// Try a transaction with a legal storage proof.
	txn.StorageProofs = append(txn.StorageProofs, StorageProof{})
	err = txn.followsStorageProofRules()
	if err != nil {
		t.Error(err)
	}

	// Try a transaction with a storage proof and a SiacoinOutput.
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, SiacoinOutput{})
	err = txn.followsStorageProofRules()
	if err != ErrStorageProofWithOutputs {
		t.Error(err)
	}
	txn.SiacoinOutputs = nil

	// Try a transaction with a storage proof and a FileContract.
	txn.FileContracts = append(txn.FileContracts, FileContract{})
	err = txn.followsStorageProofRules()
	if err != ErrStorageProofWithOutputs {
		t.Error(err)
	}
	txn.FileContracts = nil

	// Try a transaction with a storage proof and a FileContractTermination.
	txn.FileContractRevisions = append(txn.FileContractRevisions, FileContractRevision{})
	err = txn.followsStorageProofRules()
	if err != ErrStorageProofWithOutputs {
		t.Error(err)
	}
	txn.FileContractRevisions = nil

	// Try a transaction with a storage proof and a FileContractTermination.
	txn.SiafundOutputs = append(txn.SiafundOutputs, SiafundOutput{})
	err = txn.followsStorageProofRules()
	if err != ErrStorageProofWithOutputs {
		t.Error(err)
	}
	txn.SiafundOutputs = nil
}

// TestTransactionNoRepeats probes the noRepeats method of the Transaction
// type.
func TestTransactionNoRepeats(t *testing.T) {
	// Try a transaction all the repeatable types but no conflicts.
	txn := Transaction{
		SiacoinInputs:         []SiacoinInput{SiacoinInput{}},
		StorageProofs:         []StorageProof{StorageProof{}},
		FileContractRevisions: []FileContractRevision{FileContractRevision{}},
		SiafundInputs:         []SiafundInput{SiafundInput{}},
	}
	txn.FileContractRevisions[0].ParentID[0] = 1 // Otherwise it will conflict with the storage proof.
	err := txn.noRepeats()
	if err != nil {
		t.Error(err)
	}

	// Try a transaction double spending a siacoin output.
	txn.SiacoinInputs = append(txn.SiacoinInputs, SiacoinInput{})
	err = txn.noRepeats()
	if err != ErrDoubleSpend {
		t.Error(err)
	}
	txn.SiacoinInputs = txn.SiacoinInputs[:1]

	// Try double spending a file contract, checking that both storage proofs
	// and terminations can conflict with each other.
	txn.StorageProofs = append(txn.StorageProofs, StorageProof{})
	err = txn.noRepeats()
	if err != ErrDoubleSpend {
		t.Error(err)
	}
	txn.StorageProofs = txn.StorageProofs[:1]

	// Have the storage proof conflict with the file contract termination.
	txn.StorageProofs[0].ParentID[0] = 1
	err = txn.noRepeats()
	if err != ErrDoubleSpend {
		t.Error(err)
	}
	txn.StorageProofs[0].ParentID[0] = 0

	// Have the file contract termination conflict with itself.
	txn.FileContractRevisions = append(txn.FileContractRevisions, FileContractRevision{})
	txn.FileContractRevisions[1].ParentID[0] = 1
	err = txn.noRepeats()
	if err != ErrDoubleSpend {
		t.Error(err)
	}
	txn.FileContractRevisions = txn.FileContractRevisions[:1]

	// Try a transaction double spending a siafund output.
	txn.SiafundInputs = append(txn.SiafundInputs, SiafundInput{})
	err = txn.noRepeats()
	if err != ErrDoubleSpend {
		t.Error(err)
	}
	txn.SiafundInputs = txn.SiafundInputs[:1]
}

// TestValudUnlockConditions probes the validUnlockConditions function.
func TestValidUnlockConditions(t *testing.T) {
	// The only thing to check is the timelock.
	uc := UnlockConditions{Timelock: 3}
	err := validUnlockConditions(uc, 2)
	if err != ErrTimelockNotSatisfied {
		t.Error(err)
	}
	err = validUnlockConditions(uc, 3)
	if err != nil {
		t.Error(err)
	}
	err = validUnlockConditions(uc, 4)
	if err != nil {
		t.Error(err)
	}
}

// TestTransactionValidUnlockConditions probes the validUnlockConditions method
// of the transaction type.
func TestTransactionValidUnlockConditions(t *testing.T) {
	// Create a transaction with each type of valid unlock condition.
	txn := Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{
				UnlockConditions: UnlockConditions{Timelock: 3},
			},
		},
		FileContractRevisions: []FileContractRevision{
			FileContractRevision{
				UnlockConditions: UnlockConditions{Timelock: 3},
			},
		},
		SiafundInputs: []SiafundInput{
			SiafundInput{
				UnlockConditions: UnlockConditions{Timelock: 3},
			},
		},
	}
	err := txn.validUnlockConditions(4)
	if err != nil {
		t.Error(err)
	}

	// Try with illegal conditions in the siacoin inputs.
	txn.SiacoinInputs[0].UnlockConditions.Timelock = 5
	err = txn.validUnlockConditions(4)
	if err == nil {
		t.Error(err)
	}
	txn.SiacoinInputs[0].UnlockConditions.Timelock = 3

	// Try with illegal conditions in the siafund inputs.
	txn.FileContractRevisions[0].UnlockConditions.Timelock = 5
	err = txn.validUnlockConditions(4)
	if err == nil {
		t.Error(err)
	}
	txn.FileContractRevisions[0].UnlockConditions.Timelock = 3

	// Try with illegal conditions in the siafund inputs.
	txn.SiafundInputs[0].UnlockConditions.Timelock = 5
	err = txn.validUnlockConditions(4)
	if err == nil {
		t.Error(err)
	}
	txn.SiafundInputs[0].UnlockConditions.Timelock = 3
}

// TestTransactionStandaloneValid probes the StandaloneValid method of the
// Transaction type.
func TestTransactionStandaloneValid(t *testing.T) {
	// Build a working transaction.
	var txn Transaction
	err := txn.StandaloneValid(0)
	if err != nil {
		t.Error(err)
	}

	// Violate fitsInABlock.
	data := make([]byte, BlockSizeLimit)
	txn.ArbitraryData = []string{string(data)}
	err = txn.StandaloneValid(0)
	if err == nil {
		t.Error("failed to trigger fitsInABlock error")
	}
	txn.ArbitraryData = nil

	// Violate followsStorageProofRules
	txn.StorageProofs = []StorageProof{StorageProof{}}
	txn.SiacoinOutputs = []SiacoinOutput{SiacoinOutput{}}
	txn.SiacoinOutputs[0].Value = NewCurrency64(1)
	err = txn.StandaloneValid(0)
	if err == nil {
		t.Error("failed to trigger followsStorageProofRules error")
	}
	txn.StorageProofs = nil
	txn.SiacoinOutputs = nil

	// Violate noRepeats
	txn.SiacoinInputs = []SiacoinInput{SiacoinInput{}, SiacoinInput{}}
	err = txn.StandaloneValid(0)
	if err == nil {
		t.Error("failed to trigger noRepeats error")
	}
	txn.SiacoinInputs = nil

	// Violate followsMinimumValues
	txn.SiacoinOutputs = []SiacoinOutput{SiacoinOutput{}}
	err = txn.StandaloneValid(0)
	if err == nil {
		t.Error("failed to trigger followsMinimumValues error")
	}
	txn.SiacoinOutputs = nil

	// Violate correctFileContracts
	txn.FileContracts = []FileContract{
		FileContract{
			Payout:      NewCurrency64(1),
			WindowStart: 5,
			WindowEnd:   5,
		},
	}
	err = txn.StandaloneValid(0)
	if err == nil {
		t.Error("failed to trigger correctFileContracts error")
	}
	txn.FileContracts = nil

	// Violate validUnlockConditions
	txn.SiacoinInputs = []SiacoinInput{SiacoinInput{}}
	txn.SiacoinInputs[0].UnlockConditions.Timelock = 1
	err = txn.StandaloneValid(0)
	if err == nil {
		t.Error("failed to trigger validUnlockConditions error")
	}
	txn.SiacoinInputs = nil

	// Violate validSignatures
	txn.TransactionSignatures = []TransactionSignature{TransactionSignature{}}
	err = txn.StandaloneValid(0)
	if err == nil {
		t.Error("failed to trigger validSignatures error")
	}
	txn.TransactionSignatures = nil
}
