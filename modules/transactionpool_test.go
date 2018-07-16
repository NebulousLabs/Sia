package modules

import (
	"testing"

	"gitlab.com/NebulousLabs/Sia/encoding"
	"gitlab.com/NebulousLabs/Sia/types"
)

// TestConsensusConflict checks that the consensus conflict type is correctly
// assembling consensus conflict errors.
func TestConsensusConflict(t *testing.T) {
	t.Parallel()

	ncc := NewConsensusConflict("problem")
	if ncc.Error() != "consensus conflict: problem" {
		t.Error("wrong error message being reported in a consensus conflict")
	}

	err := func() error {
		return ncc
	}()
	if err.Error() != "consensus conflict: problem" {
		t.Error("wrong error message being reported in a consensus conflict")
	}
	if _, ok := err.(ConsensusConflict); !ok {
		t.Error("error is not maintaining consensus conflict type")
	}
}

// TestCalculateFee checks that the CalculateFee function is correctly tallying
// the number of fees in a transaction set.
func TestCalculateFee(t *testing.T) {
	t.Parallel()

	// Try calculating the fees on a nil transaction set.
	if CalculateFee(nil).Cmp(types.ZeroCurrency) != 0 {
		t.Error("CalculateFee is incorrectly handling nil input")
	}
	// Try a single transaction with no fees.
	txnSet := []types.Transaction{{}}
	if CalculateFee(txnSet).Cmp(types.ZeroCurrency) != 0 {
		t.Error("CalculateFee is not correctly calculating the fees on an empty transaction set")
	}
	// Try a non-empty transaction.
	txnSet = []types.Transaction{{
		SiacoinOutputs: []types.SiacoinOutput{{
			Value: types.NewCurrency64(253e9),
		}},
	}}
	if CalculateFee(txnSet).Cmp(types.ZeroCurrency) != 0 {
		t.Error("CalculateFee is not correctly calculating the fees on a non-empty transaction set")
	}

	// Try a transaction set with a single miner fee.
	baseFee := types.NewCurrency64(12e3)
	txnSet = []types.Transaction{{
		MinerFees: []types.Currency{
			baseFee,
		},
	}}
	setLen := uint64(len(encoding.Marshal(txnSet)))
	expectedFee := baseFee.Div64(setLen)
	if CalculateFee(txnSet).Cmp(expectedFee) != 0 {
		t.Error("CalculateFee doesn't seem to be calculating the correct transaction fee")
	}
	// Try the transaction set when there is more data.
	txnSet[0].FileContracts = append(txnSet[0].FileContracts, types.FileContract{
		FileSize: 10e3,
	})
	newSetLen := uint64(len(encoding.Marshal(txnSet)))
	if newSetLen <= setLen {
		t.Fatal("transaction set did not grow after adding a file contract")
	}
	newExpectedFee := baseFee.Div64(newSetLen)
	if newExpectedFee.Cmp(expectedFee) >= 0 {
		t.Error("the new expected fee should go down as the txn size increases")
	}
	if CalculateFee(txnSet).Cmp(newExpectedFee) != 0 {
		t.Error("the new expected fee does not match the new actual fee")
	}

	// Try a transaction set with multiple transactions and multiple fees per
	// transaction.
	fee1 := types.NewCurrency64(1e6)
	fee2 := types.NewCurrency64(2e6)
	fee3 := types.NewCurrency64(3e6)
	fee4 := types.NewCurrency64(4e6)
	txnSet = []types.Transaction{
		{
			MinerFees: []types.Currency{
				fee1,
				fee2,
			},
		},
		{
			MinerFees: []types.Currency{
				fee3,
				fee4,
			},
		},
	}
	currencyLen := types.NewCurrency64(uint64(len(encoding.Marshal(txnSet))))
	multiExpectedFee := fee1.Add(fee2).Add(fee3).Add(fee4).Div(currencyLen)
	if CalculateFee(txnSet).Cmp(multiExpectedFee) != 0 {
		t.Error("got the wrong fee for a multi transaction set")
	}
}
