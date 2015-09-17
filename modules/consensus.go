package modules

import (
	"errors"

	"github.com/NebulousLabs/Sia/types"
)

const (
	ConsensusDir = "consensus"

	// DiffApply and DiffRevert are the names given to the variables
	// corresponding to applying and reverting diffs.
	DiffApply  DiffDirection = true
	DiffRevert DiffDirection = false
)

var (
	ErrBlockKnown        = errors.New("block already present in database")
	ErrNonExtendingBlock = errors.New("block does not extend the longest fork")
)

type (
	// A DiffDirection indicates the "direction" of a diff, either applied or
	// reverted. A bool is used to restrict the value to these two possibilities.
	DiffDirection bool

	// A ConsensusSetSubscriber is an object that receives updates to the consensus
	// set every time there is a change in consensus.
	ConsensusSetSubscriber interface {
		// ProcessConsensusChange sends a consensus update to a module through
		// a function call. Updates will always be sent in the correct order.
		ProcessConsensusChange(ConsensusChange)
	}

	// A ConsensusChange enumerates a set of changes that occured to the consensus set.
	ConsensusChange struct {
		// RevertedBlocks is the list of blocks that were reverted by the change.
		// The reverted blocks were always all reverted before the applied blocks
		// were applied. The revered blocks are presented in the order that they
		// were reverted.
		RevertedBlocks []types.Block

		// AppliedBlocks is the list of blocks that were applied by the change. The
		// applied blocks are always all applied after all the reverted blocks were
		// reverted. The applied blocks are presented in the order that they were
		// applied.
		AppliedBlocks []types.Block

		// SiacoinOutputDiffs contains the set of siacoin diffs that were applied
		// to the consensus set in the recent change. The direction for the set of
		// diffs is 'DiffApply'.
		SiacoinOutputDiffs []SiacoinOutputDiff

		// FileContractDiffs contains the set of file contract diffs that were
		// applied to the consensus set in the recent change. The direction for the
		// set of diffs is 'DiffApply'.
		FileContractDiffs []FileContractDiff

		// SiafundOutputDiffs contains the set of siafund diffs that were applied
		// to the consensus set in the recent change. The direction for the set of
		// diffs is 'DiffApply'.
		SiafundOutputDiffs []SiafundOutputDiff

		// DelayedSiacoinOutputDiffs contains the set of delayed siacoin output
		// diffs that were applied to the consensus set in the recent change.
		DelayedSiacoinOutputDiffs []DelayedSiacoinOutputDiff

		// SiafundPoolDiffs are the siafund pool diffs that were applied to the
		// consensus set in the recent change.
		SiafundPoolDiffs []SiafundPoolDiff
	}

	// A SiacoinOutputDiff indicates the addition or removal of a SiacoinOutput in
	// the consensus set.
	SiacoinOutputDiff struct {
		Direction     DiffDirection
		ID            types.SiacoinOutputID
		SiacoinOutput types.SiacoinOutput
	}

	// A FileContractDiff indicates the addition or removal of a FileContract in
	// the consensus set.
	FileContractDiff struct {
		Direction    DiffDirection
		ID           types.FileContractID
		FileContract types.FileContract
	}

	// A SiafundOutputDiff indicates the addition or removal of a SiafundOutput in
	// the consensus set.
	SiafundOutputDiff struct {
		Direction     DiffDirection
		ID            types.SiafundOutputID
		SiafundOutput types.SiafundOutput
	}

	// A DelayedSiacoinOutputDiff indicates the introduction of a siacoin output
	// that cannot be spent until after maturing for 144 blocks. When the output
	// has matured, a SiacoinOutputDiff will be provided.
	DelayedSiacoinOutputDiff struct {
		Direction      DiffDirection
		ID             types.SiacoinOutputID
		SiacoinOutput  types.SiacoinOutput
		MaturityHeight types.BlockHeight
	}

	// A SiafundPoolDiff contains the value of the siafundPool before the block
	// was applied, and after the block was applied. When applying the diff, set
	// siafundPool to 'Adjusted'. When reverting the diff, set siafundPool to
	// 'Previous'.
	SiafundPoolDiff struct {
		Direction DiffDirection
		Previous  types.Currency
		Adjusted  types.Currency
	}

	// A ConsensusSet accepts blocks and builds an understanding of network
	// consensus.
	ConsensusSet interface {
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

		// ConsensusChange returns the ith consensus change that was broadcast to
		// subscribers by the consensus set. An error is returned if i consensus
		// changes have not been broadcast. The primary purpose of this function is
		// to rescan the blockchain.
		ConsensusChange(i int) (ConsensusChange, error)

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

		// TryTransactionSet checks whether the transaction set would be valid if
		// it were added in the next block. A consensus change is returned
		// detailing the diffs that would result from the application of the
		// transaction.
		TryTransactionSet([]types.Transaction) (ConsensusChange, error)
	}
)

// Append takes to ConsensusChange objects and adds all of their diffs together.
//
// NOTE: It is possible for diffs to overlap or be inconsistent. This function
// should only be used with consecutive or disjoint consensus change objects.
func (cc ConsensusChange) Append(cc2 ConsensusChange) ConsensusChange {
	return ConsensusChange{
		RevertedBlocks:            append(cc.RevertedBlocks, cc2.RevertedBlocks...),
		AppliedBlocks:             append(cc.AppliedBlocks, cc2.AppliedBlocks...),
		SiacoinOutputDiffs:        append(cc.SiacoinOutputDiffs, cc2.SiacoinOutputDiffs...),
		FileContractDiffs:         append(cc.FileContractDiffs, cc2.FileContractDiffs...),
		SiafundOutputDiffs:        append(cc.SiafundOutputDiffs, cc2.SiafundOutputDiffs...),
		DelayedSiacoinOutputDiffs: append(cc.DelayedSiacoinOutputDiffs, cc2.DelayedSiacoinOutputDiffs...),
	}
}
