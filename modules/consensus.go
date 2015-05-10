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

type ConsensusSet interface {
	AcceptBlock(types.Block) error

	ChildTarget(types.BlockID) (types.Target, bool)

	Close() error

	ConsensusSetSubscribe(ConsensusSetSubscriber)

	Synchronize(NetAddress) error
}
