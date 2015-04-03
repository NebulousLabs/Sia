package api

import (
	"math/big"
	"net/http"
	"time"

	"github.com/NebulousLabs/Sia/types"
)

// SiaConstants is a struct listing all of the constants in use.
type SiaConstants struct {
	BlockSizeLimit        uint64
	BlockFrequency        types.BlockHeight
	TargetWindow          types.BlockHeight
	MedianTimestampWindow int
	FutureThreshold       types.Timestamp
	SiafundCount          uint64
	SiafundPortion        float64

	InitialCoinbase uint64
	MinimumCoinbase uint64

	MaturityDelay types.BlockHeight

	GenesisTimestamp         types.Timestamp
	GenesisSiafundUnlockHash types.UnlockHash
	GenesisClaimUnlockHash   types.UnlockHash

	RootTarget types.Target
	RootDepth  types.Target

	MaxAdjustmentUp   *big.Rat
	MaxAdjustmentDown *big.Rat

	CoinbaseAugment *big.Int
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
func (srv *Server) debugConstantsHandler(w http.ResponseWriter, req *http.Request) {
	sc := SiaConstants{
		GenesisTimestamp:      types.GenesisTimestamp,
		BlockSizeLimit:        types.BlockSizeLimit,
		BlockFrequency:        types.BlockFrequency,
		TargetWindow:          types.TargetWindow,
		MedianTimestampWindow: types.MedianTimestampWindow,
		FutureThreshold:       types.FutureThreshold,
		SiafundCount:          types.SiafundCount,
		MaturityDelay:         types.MaturityDelay,
		SiafundPortion:        types.SiafundPortion,

		InitialCoinbase: types.InitialCoinbase,
		MinimumCoinbase: types.MinimumCoinbase,
		CoinbaseAugment: types.CoinbaseAugment,

		RootTarget: types.RootTarget,
		RootDepth:  types.RootDepth,

		MaxAdjustmentUp:   types.MaxAdjustmentUp,
		MaxAdjustmentDown: types.MaxAdjustmentDown,
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
func (srv *Server) mutexTestHandler(w http.ResponseWriter, req *http.Request) {
	// Call functions that result in locks but use inputs that don't result in
	// changes. After the blocking function unlocks, set the value to true.
	var mds ModuleDeadlockStatus
	go func() {
		srv.state.AcceptBlock(types.Block{})
		mds.State = true
	}()
	go func() {
		srv.gateway.RemovePeer("")
		mds.Gateway = true
	}()
	go func() {
		srv.host.Info()
		mds.Host = true
	}()
	go func() {
		srv.hostdb.Remove("")
		mds.HostDB = true
	}()
	go func() {
		srv.miner.FindBlock()
		mds.Miner = true
	}()
	go func() {
		srv.renter.Rename("", "")
		mds.Renter = true
	}()
	go func() {
		srv.tpool.AcceptTransaction(types.Transaction{})
		mds.TransactionPool = true
	}()
	go func() {
		srv.wallet.CoinAddress()
		mds.Wallet = true
	}()
	time.Sleep(time.Second * 3)

	writeJSON(w, mds)
}
