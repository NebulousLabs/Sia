package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	ConsensusDir = "consensus"
)

// A ConsensusSetSubscriber is an object that receives updates to the consensus
// set every time there is a change in consensus.
type ConsensusSetSubscriber interface {
	// ReceiveConsensusSetUpdate sends a consensus update to a module through a
	// function call. Updates will always be sent in the correct order.
	// Usually, the function receiving the updates will also process the
	// changes. If the function blocks indefinitely, the state will still
	// function.
	ReceiveConsensusSetUpdate(revertedBlocks []types.Block, appliedBlocks []types.Block)
}

// A DiffDirection indicates the "direction" of a diff, either applied or
// reverted. A bool is used to restrict the value to these two possibilities.
type DiffDirection bool

const (
	DiffApply  DiffDirection = true
	DiffRevert DiffDirection = false
)

// A SiacoinOutputDiff indicates the addition or removal of a SiacoinOutput in
// the consensus set.
type SiacoinOutputDiff struct {
	Direction     DiffDirection
	ID            types.SiacoinOutputID
	SiacoinOutput types.SiacoinOutput
}

// A FileContractDiff indicates the addition or removal of a FileContract in
// the consensus set.
type FileContractDiff struct {
	Direction    DiffDirection
	ID           types.FileContractID
	FileContract types.FileContract
}

// A SiafundOutputDiff indicates the addition or removal of a SiafundOutput in
// the consensus set.
type SiafundOutputDiff struct {
	Direction     DiffDirection
	ID            types.SiafundOutputID
	SiafundOutput types.SiafundOutput
}

// A SiafundPoolDiff contains the value of the siafundPool before the block
// was applied, and after the block was applied. When applying the diff, set
// siafundPool to 'Adjusted'. When reverting the diff, set siafundPool to
// 'Previous'.
type SiafundPoolDiff struct {
	Previous types.Currency
	Adjusted types.Currency
}

// A ConsensusSet accepts blocks and builds an understanding of network
// consensus.
type ConsensusSet interface {
	// AcceptBlock adds a block to consensus. An error will be returned if the
	// block is invalid, has been seen before, is an orphan, or doesn't
	// contribute to the heaviest fork known to the consensus set. If the block
	// does not become the head of the heaviest known fork but is otherwise
	// valid, it will be remembered by the consensus set but an error will
	// still be returned.
	AcceptBlock(types.Block) error

	// ChildTarget returns the target required to extend the current heaviest
	// fork. This function is typically used by miners looking to extend the
	// heaviest fork.
	ChildTarget(types.BlockID) (types.Target, bool)

	// Close will shut down the consensus set, giving the module enough time to
	// run any required closing routines.
	Close() error

	// ConsensusSetSubscribe will subscribe another module to the consensus
	// set. Every time that there is a change to the consensus set, an update
	// will be sent to the module via the 'ReceiveConsensusSetUpdate' function.
	// This is a thread-safe way of managing updates.
	ConsensusSetSubscribe(ConsensusSetSubscriber)

	// CurrentBlock returns the most recent block on the heaviest fork known to
	// the consensus set.
	CurrentBlock() types.Block

	// EarliestChildTimestamp returns the earliest timestamp that is acceptable
	// on the current longest fork according to the consensus set. This is a
	// required piece of information for the miner, who could otherwise be at
	// risk of mining invalid blocks.
	EarliestChildTimestamp(types.BlockID) (types.Timestamp, bool)

	// GenesisBlock returns the genesis block.
	GenesisBlock() types.Block

	// InCurrentPath returns true if the block id presented is found in the
	// current path, false otherwise.
	InCurrentPath(types.BlockID) bool

	// Synchronize will try to synchronize to a specific peer. During general
	// use, this call should never be necessary.
	Synchronize(NetAddress) error
}
