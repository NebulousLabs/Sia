package modules

const (
	// PoolDir names the directory that contains the pool persistence.
	PoolDir = "pool"
)

var (
// Whatever variables we need as we go
)

type (
	// PoolMiningMetrics stores the various stats of the pool.
	PoolMiningMetrics struct {
	}

	// PoolInternalSettings contains a list of settings that can be changed.
	PoolInternalSettings struct {
		AcceptingShares bool `json:"acceptingshares"`

		PoolOwnerPercentage float32 `json:"poolownerpercentage"`
		PoolOwnerWallet     string  `json:"poolowneraddress"`

		PoolNetworkPort uint16 `json:"poolnetworkport"`
	}

	// PoolWorkingStatus reports the working state of a pool. Can be one of
	// "starting", "accepting", or "not accepting".
	PoolWorkingStatus string

	// PoolConnectabilityStatus reports the connectability state of a pool. Can be
	// one of "checking", "connectable", or "not connectable"
	PoolConnectabilityStatus string

	// A Pool accepts incoming target solutions, tracks the share (an attempted solution),
	// checks to see if we have a new block, and if so, pays all the share submitters,
	// proportionally based on their share of the solution (minus a percentage to the
	// pool operator )
	Pool interface {
		// PoolMiningMetrics returns the mining statistics of the pool.
		MiningMetrics() PoolMiningMetrics

		// InternalSettings returns the pool's internal settings, including
		// potentially private or sensitive information.
		InternalSettings() PoolInternalSettings

		// SetInternalSettings sets the parameters of the pool.
		SetInternalSettings(PoolInternalSettings) error

		// Close closes the Pool.
		Close() error

		// ConnectabilityStatus returns the connectability status of the pool, that
		// is, if it can connect to itself on the configured NetAddress.
		ConnectabilityStatus() PoolConnectabilityStatus

		// WorkingStatus returns the working state of the pool, determined by if
		// settings calls are increasing.
		WorkingStatus() PoolWorkingStatus

		// StartPool turns on the mining pool, which will endlessly work for new
		// blocks.
		StartPool()

		// StopPool turns off the mining pool
		StopPool()

		// GetRunning returns the running status (or not) of the pool
		GetRunning() bool
	}
)
