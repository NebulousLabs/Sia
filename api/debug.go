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

	SiacoinPrecision types.Currency
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

		InitialCoinbase:  types.InitialCoinbase,
		MinimumCoinbase:  types.MinimumCoinbase,
		SiacoinPrecision: types.SiacoinPrecision,

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
func (srv *Server) debugMutextestHandler(w http.ResponseWriter, req *http.Request) {
	// Call functions that result in locks but use inputs that don't result in
	// changes. After the blocking function unlocks, set the value to true.
	var mds ModuleDeadlockStatus
	go func() {
		srv.cs.AcceptBlock(types.Block{})
		mds.State = true
	}()
	go func() {
		srv.gateway.Address()
		mds.Gateway = true
	}()
	go func() {
		srv.host.Info()
		mds.Host = true
	}()
	go func() {
		srv.hostdb.RemoveHost("")
		mds.HostDB = true
	}()
	go func() {
		srv.miner.FindBlock()
		mds.Miner = true
	}()
	go func() {
		srv.renter.RenameFile("", "")
		mds.Renter = true
	}()
	go func() {
		srv.tpool.AcceptTransaction(types.Transaction{})
		mds.TransactionPool = true
	}()
	go func() {
		srv.wallet.CoinAddress(false) // false indicates that the address should not be visible to the user.
		mds.Wallet = true
	}()
	time.Sleep(time.Second * 3)

	writeJSON(w, mds)
}

// debugSiafundsendtestHandler is a debugging tool to check that siafund
// sending works.
func (srv *Server) debugSiafundsendtestHandler(w http.ResponseWriter, req *http.Request) {
	dest2o3 := types.UnlockHash{209, 246, 228, 60, 248, 78, 242, 110, 9, 8, 227, 248, 225, 216, 163, 52, 142, 93, 47, 176, 103, 41, 137, 80, 212, 8, 132, 58, 241, 189, 2, 17}
	_, err := srv.wallet.SpendSiagSiafunds(types.NewCurrency64(25), dest2o3, []string{"siag0of1of1.siakey"})
	if err != nil {
		return
	}
	writeSuccess(w)
}

// debugSiafundsendtestHandler2 is a debugging tool to check that siafund
// sending works.
func (srv *Server) debugSiafundsendtestHandler2(w http.ResponseWriter, req *http.Request) {
	dest1o1 := types.UnlockHash{214, 166, 197, 164, 29, 201, 53, 236, 106, 239, 10, 158, 127, 131, 20, 138, 63, 221, 230, 16, 98, 247, 32, 77, 210, 68, 116, 12, 241, 89, 27, 223}
	_, err := srv.wallet.SpendSiagSiafunds(types.NewCurrency64(30), dest1o1, []string{"siag0of2of3.siakey", "siag1of2of3.siakey"})
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}
