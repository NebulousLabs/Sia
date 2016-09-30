package contractor

import (
	"github.com/NebulousLabs/Sia/build"
)

const (
	// estimatedFileContractTransactionSize provides the estimated size of
	// the average file contract in bytes.
	estimatedFileContractTransactionSize = 1200
)

var (
	// minHostsForEstimations describes the minimum number of hosts that
	// are needed to make broad estimations such as the number of sectors
	// that you can store on the network for a given allowance.
	minHostsForEstimations = func() int {
		switch build.Release {
		case "dev":
			// The number is set lower than standard so that it can
			// be reached/exceeded easily within development
			// environments, but set high enough that it's also
			// easy to fall short within the development
			// environments.
			return 5
		case "standard":
			// Hosts can have a lot of variance. Selecting too many
			// hosts will high-ball the price estimation, but users
			// shouldn't be selecting rewer hosts, and if there are
			// too few hosts being selected for estimation there is
			// a risk of underestimating the actual price, which is
			// something we'd rather avoid.
			return 10
		case "testing":
			// Testing tries to happen as fast as possible,
			// therefore tends to run with a lot fewer hosts.
			return 4
		default:
			panic("unrecognized build.Release in minHostsForEstimations")
		}
	}()
)
