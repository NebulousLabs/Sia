package main

import (
	"math/big"
	"net/http"
	"time"

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

// ModuleDeadlockStatus is a struct containing a bool for each module, 'false'
// indicating that the module is deadlocked and 'true' indicating that the
// module is not deadlocked.
type ModuleDeadlockStatus struct {
	State           bool
	Gateway         bool
	Host            bool
	HostDB          bool
	Miner           bool
	Renter          bool
	TransactionPool bool
	Wallet          bool
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

// mutexTestHandler creates an bool for each module and then calls a function
// to lock and unlock the module in a goroutine. After the function, the bool
// for that module is set to true. Deadlocked modules will retain a false
// boolean. Diagnostic results are then printed.
//
// 'false' may merely indicate that it's taking longer than 3 seconds to
// acquire a lock. For our purposes, this is deadlock, even if it may
// eventually resolve.
func (d *daemon) mutexTestHandler(w http.ResponseWriter, req *http.Request) {
	// Call functions that result in locks but use inputs that don't result in
	// changes. After the blocking function unlocks, set the value to true.
	var mds ModuleDeadlockStatus
	go func() {
		d.state.AcceptBlock(consensus.Block{})
		mds.State = true
	}()
	go func() {
		d.gateway.RemovePeer("")
		mds.Gateway = true
	}()
	go func() {
		d.host.Info()
		mds.Host = true
	}()
	go func() {
		d.hostdb.Remove("")
		mds.HostDB = true
	}()
	go func() {
		d.miner.FindBlock()
		mds.Miner = true
	}()
	go func() {
		d.renter.Rename("", "")
		mds.Renter = true
	}()
	go func() {
		d.tpool.AcceptTransaction(consensus.Transaction{})
		mds.TransactionPool = true
	}()
	go func() {
		d.wallet.CoinAddress()
		mds.Wallet = true
	}()
	time.Sleep(time.Second * 3)

	writeJSON(w, mds)
}
