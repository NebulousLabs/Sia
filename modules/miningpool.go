package modules

import (
//"math/big"
)

const (
	MiningPoolDir = "miningpool"
)

// The MiningPool interface provides functions that allow external miners
// mine for the pool
type MiningPool interface {
	// CreatePaymentChannel creates a payment channel from the MiningPool to the
	// miner. This allows for the pool to send currency to the miner off-chain
	// in order to prevent cluttering the network
	CreatePaymentChannel() error

	// Settings returns the host's settings
	Settings() MiningPoolSettings

	// Should there be some API call to communicate which side of a fork the pool is on?
}

type MiningPoolSettings struct {
	MaxConnections uint32
	TargetMultiple uint32
	//MiningPoolCut  big.Rat
	//MinerCut       big.Rat
}
