package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/network"
)

// This file is here to provide access to information about the state without
// actually needing to export the state. This allows importing packages to see
// things like state height and depth, but without giving them the ability to
// disrupt the environment's image of the state.

// Contains basic information about the state, but does not go into depth.
type StateInfo struct {
	CurrentBlock           consensus.BlockID
	Height                 consensus.BlockHeight
	Target                 consensus.Target
	Depth                  consensus.Target
	EarliestLegalTimestamp consensus.Timestamp
}

// Contains in depth information about the state - potentially a lot of
// information.
type DeepStateInfo struct {
	StateHash hash.Hash

	UtxoSet         []consensus.OutputID
	TransactionList []consensus.Transaction
}

// EnvironmentInfo contains lightweight information about the environment.
// Controvertially, instead of using canonical types, EnvironmentInfo switches
// out a few of the types to be more human readable.
type EnvironmentInfo struct {
	StateInfo StateInfo

	WalletBalance consensus.Currency
	WalletAddress string

	RenterFiles []string

	IPAddress          network.NetAddress
	HostSettings       HostAnnouncement
	HostSpaceRemaining int64
	HostContractCount  int

	Mining string
}

// StateInfo returns a bunch of useful information about the state, doing
// read-only accesses. StateInfo does not lock the state mutex, which means
// that the data could potentially be weird on account of race conditions.
// Because it's just a read-only call, it will not adversely affect the state.
// If accurate data is paramount, SafeStateInfo() should be called, though this
// can adversely affect performance.
func (e *Environment) StateInfo() StateInfo {
	e.state.RLock()
	defer e.state.RUnlock()

	return StateInfo{
		CurrentBlock: e.state.CurrentBlock().ID(),
		Height:       e.state.Height(),
		Target:       e.state.CurrentTarget(),
		Depth:        e.state.Depth(),
		EarliestLegalTimestamp: e.state.EarliestLegalTimestamp(),
	}
}

func (e *Environment) DeepStateInfo() DeepStateInfo {
	e.state.RLock()
	defer e.state.RUnlock()

	return DeepStateInfo{
		StateHash: e.state.StateHash(),

		UtxoSet:         e.state.SortedUtxoSet(),
		TransactionList: e.state.TransactionList(),
	}
}

// EnvrionmentInfo returns a bunch of simple information about the environment.
func (e *Environment) EnvironmentInfo() (eInfo EnvironmentInfo) {
	eInfo = EnvironmentInfo{
		StateInfo: e.StateInfo(),

		WalletBalance: e.WalletBalance(),

		IPAddress:          e.server.NetAddress(),
		HostSettings:       e.HostSettings(),
		HostSpaceRemaining: e.HostSpaceRemaining(),
	}

	if e.Mining() {
		eInfo.Mining = "On"
	} else {
		eInfo.Mining = "Off"
	}

	coinAddress := e.CoinAddress()
	eInfo.WalletAddress = fmt.Sprintf("%x", coinAddress)

	e.renter.RLock()
	for filename := range e.renter.Files {
		eInfo.RenterFiles = append(eInfo.RenterFiles, filename)
	}
	e.renter.RUnlock()

	e.host.RLock()
	eInfo.HostContractCount = len(e.host.Files)
	e.host.RUnlock()

	return
}

// Output returns the output that corresponds with a certain OutputID. It does
// not lock the mutex, which means it could potentially (but usually doesn't)
// produce weird or incorrect output.
func (e *Environment) Output(id consensus.OutputID) (output consensus.Output, err error) {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.Output(id)
}

func (e *Environment) Height() consensus.BlockHeight {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.Height()
}

func (e *Environment) TransactionList() []consensus.Transaction {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.TransactionList()
}

func (e *Environment) BlockFromID(bid consensus.BlockID) (consensus.Block, error) {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.BlockFromID(bid)
}

func (e *Environment) BlockAtHeight(height consensus.BlockHeight) (consensus.Block, error) {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.BlockAtHeight(height)
}

func (e *Environment) AddressBook() []network.NetAddress {
	return e.server.AddressBook()
}

func (e *Environment) RandomPeer() network.NetAddress {
	return e.server.RandomPeer()
}

func (e *Environment) NetAddress() network.NetAddress {
	return e.server.NetAddress()
}
