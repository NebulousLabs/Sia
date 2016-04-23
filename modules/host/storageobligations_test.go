package host

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestStorageObligationID checks that the return function of the storage
// obligation returns the correct value for the obligaiton id.
func TestStorageObligationID(t *testing.T) {
	t.Parallel()
	// Try a transaction set with just a file contract.
	so1 := &storageObligation{
		OriginTransactionSet: []types.Transaction{{
			FileContracts: []types.FileContract{{
				ValidProofOutputs: []types.SiacoinOutput{
					{
						UnlockHash: types.UnlockHash{2, 1, 3},
						Value:      types.NewCurrency64(35),
					},
					{
						UnlockHash: types.UnlockHash{0, 1, 3},
						Value:      types.NewCurrency64(25),
					},
				},
				MissedProofOutputs: []types.SiacoinOutput{
					{
						UnlockHash: types.UnlockHash{110, 1, 3},
						Value:      types.NewCurrency64(3325),
					},
					{
						UnlockHash: types.UnlockHash{110, 1, 3},
						Value:      types.NewCurrency64(8325),
					},
				},
			}},
		}},
	}
	if so1.id() != so1.OriginTransactionSet[0].FileContractID(0) {
		t.Error("id function of storage obligation is not correct")
	}

	// Try a file contract that includes file contract dependencies.
	so2 := &storageObligation{
		OriginTransactionSet: []types.Transaction{
			{
				SiacoinOutputs: []types.SiacoinOutput{{
					UnlockHash: types.UnlockHash{1, 3, 2},
					Value:      types.NewCurrency64(5),
				}},
			},
			{
				FileContracts: []types.FileContract{{
					ValidProofOutputs: []types.SiacoinOutput{
						{
							UnlockHash: types.UnlockHash{8, 11, 4},
							Value:      types.NewCurrency64(85),
						},
						{
							UnlockHash: types.UnlockHash{8, 11, 14},
							Value:      types.NewCurrency64(859),
						},
					},
					MissedProofOutputs: []types.SiacoinOutput{
						{
							UnlockHash: types.UnlockHash{8, 113, 4},
							Value:      types.NewCurrency64(853),
						},
						{
							UnlockHash: types.UnlockHash{8, 119, 14},
							Value:      types.NewCurrency64(9859),
						},
					},
				}},
			},
		},
	}
	if so2.id() != so2.OriginTransactionSet[1].FileContractID(0) {
		t.Error("id function of storage obligation incorrect for file contracts with dependencies")
	}
}
