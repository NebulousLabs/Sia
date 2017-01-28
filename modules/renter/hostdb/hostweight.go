package hostdb

import (
	"math/big"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// Because most weights would otherwise be fractional, we set the base
	// weight to be very large.
	baseWeight = types.NewCurrency(new(big.Int).Exp(big.NewInt(10), big.NewInt(75), nil))

	// collateralExponentiation is the number of times that the collateral is
	// multiplied into the price.
	//
	// NOTE: Changing this value downwards needs that the baseWeight will need
	// to be increased.
	collateralExponentiation = 1

	// priceDiveNormalization reduces the raw value of the price so that not so
	// many digits are needed when operating on the weight. This also allows the
	// base weight to be a lot lower.
	priceDivNormalization = types.SiacoinPrecision.Div64(100)

	// minCollateral is the amount of collateral we weight all hosts as having,
	// even if they do not have any collateral. This is to temporarily prop up
	// weak / cheap hosts on the network while the network is bootstrapping.
	minCollateral = types.SiacoinPrecision.Mul64(25)

	// Set a mimimum price, below which setting lower prices will no longer put
	// this host at an advatnage. This price is considered the bar for
	// 'essentially free', and is kept to a minimum to prevent certain Sybil
	// attack related attack vectors.
	//
	// NOTE: This needs to be intelligently adjusted down as the practical price
	// of storage changes, and as the price of the siacoin changes.
	minDivPrice = types.SiacoinPrecision.Mul64(250)

	// priceExponentiation is the number of times that the weight is divided by
	// the price.
	//
	// NOTE: Changing this value upwards means that the baseWeight will need to
	// be increased.
	priceExponentiation = 4

	// requiredStorage indicates the amount of storage that the host must be
	// offering in order to be considered a valuable/worthwhile host.
	requiredStorage = func() uint64 {
		switch build.Release {
		case "dev":
			return 1e6
		case "standard":
			return 5e9
		case "testing":
			return 1e3
		default:
			panic("incorrect/missing value for requiredStorage constant")
		}
	}()

	// uptimeExponentiation is the number of times the uptime percentage is
	// multiplied by itself when determining host uptime penalty.
	uptimeExponentiation = 18
)

// collateralAdjustments improves the host's weight according to the amount of
// collateral that they have provided.
//
// NOTE: For any reasonable value of collateral, there will be a huge blowup,
// allowing for the base weight to be a lot lower, as the collateral is
// accounted for before anything else.
func collateralAdjustments(entry modules.HostDBEntry, weight types.Currency) types.Currency {
	usedCollateral := entry.Collateral
	if entry.Collateral.Cmp(minCollateral) < 0 {
		usedCollateral = minCollateral
	}
	for i := 0; i < collateralExponentiation; i++ {
		weight = weight.Mul(usedCollateral)
	}
	return weight
}

// priceAdjustments will adjust the weight of the entry according to the prices
// that it has set.
func priceAdjustments(entry modules.HostDBEntry, weight types.Currency) types.Currency {
	// Sanity checks - the constants values need to have certain relationships
	// to eachother
	if build.DEBUG {
		// If the minDivPrice is not much larger than the divNormalization,
		// there will be problems with granularity after the divNormalization is
		// applied.
		if minDivPrice.Div64(100).Cmp(priceDivNormalization) < 0 {
			build.Critical("Maladjusted minDivePrice and divNormalization constants in hostdb package")
		}
	}

	// Prices tiered as follows:
	//    - the storage price is presented as 'per block per byte'
	//    - the contract price is presented as a flat rate
	//    - the upload bandwidth price is per byte
	//    - the download bandwidth price is per byte
	//
	// The hostdb will naively assume the following for now:
	//    - each contract covers 6 weeks of storage (default is 12 weeks, but
	//      renewals occur at midpoint) - 6048 blocks - and 10GB of storage.
	//    - uploads happen once per 12 weeks (average lifetime of a file is 12 weeks)
	//    - downloads happen once per 6 weeks (files are on average downloaded twice throughout lifetime)
	//
	// In the future, the renter should be able to track average user behavior
	// and adjust accordingly. This flexibility will be added later.
	adjustedContractPrice := entry.ContractPrice.Div64(6048).Div64(10e9) // Adjust contract price to match 10GB for 6 weeks.
	adjustedUploadPrice := entry.UploadBandwidthPrice.Div64(24192)       // Adjust upload price to match a single upload over 24 weeks.
	adjustedDownloadPrice := entry.DownloadBandwidthPrice.Div64(12096)   // Adjust download price to match one download over 12 weeks.
	siafundFee := adjustedContractPrice.Add(adjustedUploadPrice).Add(adjustedDownloadPrice).Add(entry.Collateral).MulTax()
	totalPrice := entry.StoragePrice.Add(adjustedContractPrice).Add(adjustedUploadPrice).Add(adjustedDownloadPrice).Add(siafundFee)

	// Set the divPrice, which is closely related to the totalPrice, but
	// adjusted both to make the math more computationally friendly and also
	// given a hard minimum to prevent certain classes of Sybil attacks -
	// attacks where the attacker tries to esacpe the need to burn coins by
	// setting an extremely low price.
	divPrice := totalPrice
	if divPrice.Cmp(minDivPrice) < 0 {
		divPrice = minDivPrice
	}
	// Shrink the div price so that the math can be a lot less intense. Without
	// this step, the base price would need to be closer to 10e150 as opposed to
	// 10e50.
	divPrice = divPrice.Div(priceDivNormalization)
	for i := 0; i < priceExponentiation; i++ {
		weight = weight.Div(divPrice)
	}
	return weight
}

// storageRemainingAdjustments adjusts the weight of the entry according to how
// much storage it has remaining.
func storageRemainingAdjustments(entry modules.HostDBEntry) float64 {
	base := float64(1)
	if entry.RemainingStorage < 200*requiredStorage {
		base = base / 2 // 2x total penalty
	}
	if entry.RemainingStorage < 100*requiredStorage {
		base = base / 3 // 6x total penalty
	}
	if entry.RemainingStorage < 50*requiredStorage {
		base = base / 4 // 24x total penalty
	}
	if entry.RemainingStorage < 25*requiredStorage {
		base = base / 5 // 95x total penalty
	}
	if entry.RemainingStorage < 10*requiredStorage {
		base = base / 6 // 570x total penalty
	}
	if entry.RemainingStorage < 5*requiredStorage {
		base = base / 10 // 5,700x total penalty
	}
	if entry.RemainingStorage < requiredStorage {
		base = base / 100 // 570,000x total penalty
	}
	return base
}

// versionAdjustments will adjust the weight of the entry according to the siad
// version reported by the host.
func versionAdjustments(entry modules.HostDBEntry) float64 {
	base := float64(1)
	if build.VersionCmp(entry.Version, "1.0.3") < 0 {
		base = base / 5 // 5x total penalty.
	}
	if build.VersionCmp(entry.Version, "1.0.0") < 0 {
		base = base / 20 // 100x total penalty.
	}
	return base
}

// lifetimeAdjustments will adjust the weight of the host according to the total
// amount of time that has passed since the host's original announcement.
func (hdb *HostDB) lifetimeAdjustments(entry modules.HostDBEntry) float64 {
	base := float64(1)
	if hdb.blockHeight >= entry.FirstSeen {
		age := hdb.blockHeight - entry.FirstSeen
		if age < 6000 {
			base = base / 2 // 2x total
		}
		if age < 4000 {
			base = base / 2 // 4x total
		}
		if age < 2000 {
			base = base / 4 // 16x total
		}
		if age < 1000 {
			base = base / 4 // 64x total
		}
		if age < 288 {
			base = base / 2 // 128x total
		}
	} else {
		// Shouldn't happen, but the usecase is covered anyway.
		base = base / 1000 // Because something weird is happening, don't trust this host very much.
		hdb.log.Critical("Hostdb has witnessed a host where the FirstSeen height is higher than the current block height.")
	}
	return base
}

// uptimeAdjustments penalizes the host for having poor uptime, and for being
// offline.
//
// CAUTION: The function 'managedUpdateEntry' will manually fill out two scans
// for a new host to give the host some initial uptime or downtime. Modification
// of this function needs to be made paying attention to the structure of that
// function.
func (hdb *HostDB) uptimeAdjustments(entry modules.HostDBEntry) float64 {
	// Special case: if we have scanned the host twice or fewer, don't perform
	// uptime math.
	if len(entry.ScanHistory) == 0 {
		return 0.001 // Shouldn't happen.
	}
	if len(entry.ScanHistory) == 1 {
		if entry.ScanHistory[0].Success {
			return 0.75
		}
		return 0.25
	}
	if len(entry.ScanHistory) == 2 {
		if entry.ScanHistory[0].Success && entry.ScanHistory[1].Success {
			return 0.85
		}
		if entry.ScanHistory[0].Success || entry.ScanHistory[1].Success {
			return 0.50
		}
		return 0.05
	}

	// Compute the total measured uptime and total measured downtime for this
	// host.
	var uptime time.Duration
	var downtime time.Duration
	recentTime := entry.ScanHistory[0].Timestamp
	recentSuccess := entry.ScanHistory[0].Success
	for _, scan := range entry.ScanHistory[1:] {
		if recentTime.After(scan.Timestamp) {
			hdb.log.Critical("Host entry scan history not sorted.")
		}
		if recentSuccess {
			uptime += scan.Timestamp.Sub(recentTime)
		} else {
			downtime += scan.Timestamp.Sub(recentTime)
		}
		recentTime = scan.Timestamp
		recentSuccess = scan.Success
	}
	// Sanity check against 0 total time.
	if uptime == 0 && downtime == 0 {
		return 0.001 // Shouldn't happen.
	}

	// Calculate the penalty for low uptime.
	uptimePenalty := float64(1)
	uptimeRatio := float64(uptime) / float64(uptime+downtime)
	if uptimeRatio > 0.97 {
		uptimeRatio = 0.97
	}
	uptimeRatio += 0.03
	for i := 0; i < uptimeExponentiation; i++ {
		uptimePenalty *= uptimeRatio
	}

	// Calculate the penalty for downtime across consecutive scans.
	scanLen := len(entry.ScanHistory)
	for i := scanLen - 1; i >= 0; i-- {
		if entry.ScanHistory[i].Success {
			break
		}
		uptimePenalty = uptimePenalty / 2
	}
	return uptimePenalty
}

// calculateHostWeight returns the weight of a host according to the settings of
// the host database entry. Currently, only the price is considered.
func (hdb *HostDB) calculateHostWeight(entry modules.HostDBEntry) types.Currency {
	// Perform the high resolution adjustments.
	weight := baseWeight
	weight = collateralAdjustments(entry, weight)
	weight = priceAdjustments(entry, weight)

	// Perform the lower resolution adjustments.
	storageRemainingPenalty := storageRemainingAdjustments(entry)
	versionPenalty := versionAdjustments(entry)
	lifetimePenalty := hdb.lifetimeAdjustments(entry)
	uptimePenalty := hdb.uptimeAdjustments(entry)

	// Combine the adjustments.
	fullPenalty := storageRemainingPenalty * versionPenalty * lifetimePenalty * uptimePenalty
	weight = weight.MulFloat(fullPenalty)

	if weight.IsZero() {
		// A weight of zero is problematic for for the host tree.
		return types.NewCurrency64(1)
	}
	return weight
}
