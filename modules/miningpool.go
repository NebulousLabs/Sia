package modules

const (
	MiningPoolDir = "miningpool"
)

// The MiningPool interface provides functions that allow external miners
// mine for the pool
type MiningPool interface {
	// Settings returns the host's settings
	Settings() MiningPoolSettings

	// Should there be some API call to communicate which side of a fork the pool is on?
}

type MiningPoolSettings struct {
	// The target multiple for this specfic pool
	TargetMultiple uint32

	//MiningPoolCut  big.Rat
	//MinerCut       big.Rat
}
