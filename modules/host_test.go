package modules

import (
	"math"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestUnitMaxFileContractSetLenSanity checks that a sensible value for
// MaxFileContractSetLen has been chosen.
func TestUnitMaxFileContractSetLenSanity(t *testing.T) {
	// It does not make sense for the contract set limit to be higher than the
	// IsStandard limit in the transaction pool. Such a transaction set would
	// never be accepted by the transaction pool, and therefore is going to
	// cause a failure later on in the host process. An extra 1kb is left
	// because the file contract transaction is going to grow as the terms are
	// negotiated and as signatures are added.
	if MaxFileContractSetLen > TransactionSetSizeLimit-1e3 {
		t.Fatal("MaxfileContractSetLen does not have a sensible value - should be smaller than the TransactionSetSizeLimit")
	}

}

// TestUnitStoragePriceConversions checks the functions StoragePriceToHuman and
// StoragePriceToConsensus, verifiying that they correclty convert between
// human-readable prices and consensus-level prices.
func TestUnitStoragePriceConversions(t *testing.T) {
	// Establish a series of trials for conversion.
	trials := []struct {
		consensus types.Currency
		human     uint64
	}{
		{
			// Convert 0.
			types.ZeroCurrency,
			0,
		},
		{
			// Convert from 1e12 - simple result.
			types.NewCurrency64(1e12),
			4320,
		},
		{
			// Convert from a tiny human number.
			types.NewCurrency64(6250e6),
			27,
		},
		{
			// Convert from types.SiacoinPrecision - simple result.
			types.SiacoinPrecision,
			4320e12,
		},
	}

	// Run all of the trials.
	for i, trial := range trials {
		// Convert from the consensus result to the human result, and
		// vice-versa. The transformations should be communitive, so both
		// passing should indicate that the trial has succeeded.
		toHuman, err := StoragePriceToHuman(trial.consensus)
		if err != nil {
			t.Error(i, err)
		}
		if toHuman != trial.human {
			t.Error("StoragePriceToHuman conversion failed:", i, trial.consensus, trial.human, toHuman)
		}
		if StoragePriceToConsensus(trial.human).Cmp(trial.consensus) != 0 {
			t.Error("StoragePriceToConsensus conversion failed:", i, trial.human, trial.consensus, StoragePriceToConsensus(trial.human))
		}
	}

	// Check the corner cases for StoragePriceToHuman - rounding to 1, rounding
	// to 0, and overflowing.
	causeRoundToZero := types.NewCurrency64(115740740)
	causeRoundToOne := types.NewCurrency64(115740741)
	notOverflow := types.NewCurrency64(math.MaxUint64).Mul(types.NewCurrency64(231481481))
	causeOverflow := types.NewCurrency64(math.MaxUint64).Mul(types.NewCurrency64(231481482))
	toHuman, err := StoragePriceToHuman(causeRoundToZero)
	if err != nil {
		t.Error(err)
	}
	if toHuman != 0 {
		t.Error("rounding is not happening correctly in StoragePriceToHuman")
	}
	toHuman, err = StoragePriceToHuman(causeRoundToOne)
	if err != nil {
		t.Error(err)
	}
	if toHuman != 1 {
		t.Error("rounding is not happening correctly in StoragePriceToHuman")
	}
	toHuman, err = StoragePriceToHuman(notOverflow)
	if err != nil {
		t.Error(err)
	}
	toHuman, err = StoragePriceToHuman(causeOverflow)
	if err != types.ErrUint64Overflow {
		t.Error(err)
	}
}
