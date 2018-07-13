package contractor

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Constants related to contract formation parameters.
var (
	// consecutiveRenewalsBeforeReplacement is the number of times a contract
	// attempt to be renewed before it is marked as !goodForRenew.
	consecutiveRenewalsBeforeReplacement = build.Select(build.Var{
		Dev:      types.BlockHeight(12),
		Standard: types.BlockHeight(12), // ~2h
		Testing:  types.BlockHeight(12),
	}).(types.BlockHeight)

	// fileContractMinimumFunding is the lowest percentage of an allowace (on a
	// per-contract basis) that is allowed to go into funding a contract. If the
	// allowance is 100 SC per contract (5,000 SC total for 50 contracts, or
	// 2,000 SC total for 20 contracts, etc.), then the minimum amount of funds
	// that a contract would be allowed to have is fileContractMinimumFunding *
	// 100SC.
	fileContractMinimumFunding = float64(0.15)

	// minContractFundRenewalThreshold defines the ratio of remaining funds to
	// total contract cost below which the contractor will prematurely renew a
	// contract.
	minContractFundRenewalThreshold = float64(0.03) // 3%

	// randomHostsBufferForScore defines how many extra hosts are queried when trying
	// to figure out an appropriate minimum score for the hosts that we have.
	randomHostsBufferForScore = build.Select(build.Var{
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
	maxStoragePrice  = build.Select(build.Var{
		Dev:      types.SiacoinPrecision.Mul64(30e4).Div(modules.BlockBytesPerMonthTerabyte), // 1 order of magnitude greater
		Standard: types.SiacoinPrecision.Mul64(30e3).Div(modules.BlockBytesPerMonthTerabyte), // 30k SC / TB / Month
		Testing:  types.SiacoinPrecision.Mul64(30e5).Div(modules.BlockBytesPerMonthTerabyte), // 2 orders of magnitude greater
	}).(types.Currency)
	maxUploadPrice = build.Select(build.Var{
		Dev:      maxStoragePrice.Mul64(30 * 4320),  // 1 order of magnitude greater
		Standard: maxStoragePrice.Mul64(3 * 4320),   // 3 months of storage
		Testing:  maxStoragePrice.Mul64(300 * 4320), // 2 orders of magnitude greater
	}).(types.Currency)

	// scoreLeeway defines the factor by which a host can miss the goal score
	// for a set of hosts. To determine the goal score, a new set of hosts is
	// queried from the hostdb and the lowest scoring among them is selected.
	// That score is then divided by scoreLeeway to get the minimum score that a
	// host is allowed to have before being marked as !GoodForUpload.
	scoreLeeway = types.NewCurrency64(100)
)
