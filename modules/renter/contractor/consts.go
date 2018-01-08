package contractor

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Constants related to contract formation parameters.
var (
	// To alleviate potential block propagation issues, the contractor sleeps
	// between each contract formation.
	contractFormationInterval = build.Select(build.Var{
		Dev:      10 * time.Second,
		Standard: 60 * time.Second,
		Testing:  10 * time.Millisecond,
	}).(time.Duration)

	// minContractFundRenewalThreshold defines the ratio of remaining funds to
	// total contract cost below which the contractor will prematurely renew a
	// contract.
	minContractFundRenewalThreshold = float64(0.03) // 3%

	// minScoreHostBuffer defines how many extra hosts are queried when trying
	// to figure out an appropriate minimum score for the hosts that we have.
	minScoreHostBuffer = build.Select(build.Var{
		Dev:      2,
		Standard: 10,
		Testing:  1,
	}).(int)
)

// Constants related to the safety values for when the contractor is forming
// contracts.
var (
	maxCollateral    = types.SiacoinPrecision.Mul64(1e3) // 1k SC
	maxDownloadPrice = maxStoragePrice.Mul64(3 * 4320)
	maxStoragePrice  = types.SiacoinPrecision.Mul64(30e3).Div(modules.BlockBytesPerMonthTerabyte) // 30k SC / TB / Month
	maxUploadPrice   = maxStoragePrice.Mul64(3 * 4320)                                            // 3 months of storage

	// scoreLeeway defines the factor by which a host can miss the goal score
	// for a set of hosts. To determine the goal score, a new set of hosts is
	// queried from the hostdb and the lowest scoring among them is selected.
	// That score is then divided by scoreLeeway to get the minimum score that a
	// host is allowed to have before being marked as !GoodForUpload.
	scoreLeeway = types.NewCurrency64(100)
)
