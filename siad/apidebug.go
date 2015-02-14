package main

import (
	"math/big"
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

// SiaConstants is a struct listing all of the constants in use.
type SiaConstants struct {
	GenesisTimestamp      consensus.Timestamp
	BlockSizeLimit        int
	BlockFrequency        int
	TargetWindow          int
	MedianTimestampWindow int
	FutureThreshold       int
	SiafundCount          int
	MaturityDelay         int
	SiafundPortion        float64

	InitialCoinbase int
	MinimumCoinbase int
	CoinbaseAugment *big.Int

	RootTarget consensus.Target
	RootDepth  consensus.Target

	MaxAdjustmentUp   *big.Rat
	MaxAdjustmentDown *big.Rat
}

// debugConstantsHandler prints a json file containing all of the constants.
func (d *daemon) debugConstantsHandler(w http.ResponseWriter, req *http.Request) {
	sc := SiaConstants{
		GenesisTimestamp:      consensus.GenesisTimestamp,
		BlockSizeLimit:        consensus.BlockSizeLimit,
		BlockFrequency:        consensus.BlockFrequency,
		TargetWindow:          consensus.TargetWindow,
		MedianTimestampWindow: consensus.MedianTimestampWindow,
		FutureThreshold:       consensus.FutureThreshold,
		SiafundCount:          consensus.SiafundCount,
		MaturityDelay:         consensus.MaturityDelay,
		SiafundPortion:        consensus.SiafundPortion,

		InitialCoinbase: consensus.InitialCoinbase,
		MinimumCoinbase: consensus.MinimumCoinbase,
		CoinbaseAugment: consensus.CoinbaseAugment,

		RootTarget: consensus.RootTarget,
		RootDepth:  consensus.RootDepth,

		MaxAdjustmentUp:   consensus.MaxAdjustmentUp,
		MaxAdjustmentDown: consensus.MaxAdjustmentDown,
	}

	writeJSON(w, sc)
}

func (d *daemon) mutexTestHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: Bring back.
	// d.core.ScanMutexes()
}
