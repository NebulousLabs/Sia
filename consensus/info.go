package consensus

import (
	"errors"
	"math/big"
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
)

// StateInfo contains basic information about the State.
type StateInfo struct {
	CurrentBlock BlockID
	Height       BlockHeight
	Target       Target
}

// blockAtHeight returns the block on the current path with the given height.
func (s *State) blockAtHeight(height BlockHeight) (b Block, exists bool) {
	bn, exists := s.blockMap[s.currentPath[height]]
	if !exists {
		return
	}
	b = bn.block
	return
}

// currentBlockNode returns the blockNode of the current block.
func (s *State) currentBlockNode() *blockNode {
	return s.blockMap[s.currentBlockID]
}

// currentBlockWeight returns the weight of the current block.
func (s *State) currentBlockWeight() *big.Rat {
	return s.currentBlockNode().target.Inverse()
}

// height returns the current height of the state.
func (s *State) height() BlockHeight {
	return s.blockMap[s.currentBlockID].height
}

// output returns the unspent SiacoinOutput associated with the given ID. If
// the output is not in the UTXO set, 'exists' will be false.
func (s *State) output(id SiacoinOutputID) (sco SiacoinOutput, exists bool) {
	sco, exists = s.siacoinOutputs[id]
	return
}

// sortedUscoSet returns all of the unspent siacoin outputs sorted
// according to the numerical value of their id.
func (s *State) sortedUscoSet() []SiacoinOutput {
	// Get all of the outputs in string form and sort the strings.
	unspentOutputs := make(crypto.HashSlice, len(s.siacoinOutputs))
	for outputID := range s.siacoinOutputs {
		unspentOutputs = append(unspentOutputs, crypto.Hash(outputID))
	}
	sort.Sort(unspentOutputs)

	// Get the outputs in order according to their sorted form.
	sortedOutputs := make([]SiacoinOutput, len(unspentOutputs))
	for i, outputID := range unspentOutputs {
		output, _ := s.output(SiacoinOutputID(outputID))
		sortedOutputs[i] = output
	}
	return sortedOutputs
}

// Sorted UsfoSet returns all of the unspent siafund outputs sorted according
// to the numerical value of their id.
func (s *State) sortedUsfoSet() []SiafundOutput {
	// Get all of the outputs in string form and sort the strings.
	outputIDs := make(crypto.HashSlice, len(s.siafundOutputs))
	for outputID := range s.siafundOutputs {
		outputIDs = append(outputIDs, crypto.Hash(outputID))
	}
	sort.Sort(outputIDs)

	// Get the outputs in order according to their sorted string form.
	sortedOutputs := make([]SiafundOutput, len(outputIDs))
	for i, outputID := range outputIDs {
		// Sanity check - the output should exist.
		output, exists := s.siafundOutputs[SiafundOutputID(outputID)]
		if DEBUG {
			if !exists {
				panic("output doesn't exist")
			}
		}

		sortedOutputs[i] = output
	}
	return sortedOutputs
}

// Hash returns the Markle root of the current consensus set.
func (s *State) Hash() crypto.Hash {
	// Items of interest:
	// 1.	genesis block
	// 2.	current block ID
	// 3.	current height
	// 4.	current target
	// 5.	current depth
	// 6.	earliest allowed timestamp of next block
	// 7.	current path, ordered by height.
	// 8.	unspent siacoin outputs, sorted by id.
	// 9.	open file contracts, sorted by id.
	// 10.	unspent siafund outputs, sorted by id.
	// 11.	delayed siacoin outputs, sorted by height, then sorted by id.

	// Create a slice of hashes representing all items of interest.
	leaves := []crypto.Hash{
		crypto.HashObject(s.blockRoot.block),
		crypto.Hash(s.currentBlockID),
		crypto.HashObject(s.height()),
		crypto.HashObject(s.currentBlockNode().target),
		crypto.HashObject(s.currentBlockNode().depth),
		crypto.HashObject(s.currentBlockNode().earliestChildTimestamp()),
	}

	// Add all the blocks in the current path.
	for i := 0; i < len(s.currentPath); i++ {
		leaves = append(leaves, crypto.Hash(s.currentPath[BlockHeight(i)]))
	}

	// Add the (sorted) set of siacoin outputs.
	for _, output := range s.sortedUscoSet() {
		leaves = append(leaves, crypto.HashObject(output))
	}

	// Sort the open contracts by the string value of their ID.
	openContractIDs := make(crypto.HashSlice, len(s.fileContracts))
	for contractID := range s.fileContracts {
		openContractIDs = append(openContractIDs, crypto.Hash(contractID))
	}
	sort.Sort(openContractIDs)

	// Add the open contracts in sorted order.
	for _, contractID := range openContractIDs {
		fc := s.fileContracts[FileContractID(contractID)]
		leaves = append(leaves, crypto.HashObject(fc))
	}

	// Add the (sorted) set of siafund outputs.
	for _, output := range s.sortedUsfoSet() {
		leaves = append(leaves, crypto.HashObject(output))
	}

	// Add the set of delayed siacoin outputs. The outputs are sorted first by
	// their maturity height, and then by ID.
	for i := BlockHeight(0); i <= s.height(); i++ {
		delayedOutputs := s.delayedSiacoinOutputs[i]
		delayedIDs := make(crypto.HashSlice, len(delayedOutputs))
		for id := range delayedOutputs {
			delayedIDs = append(delayedIDs, crypto.Hash(id))
		}
		sort.Sort(delayedIDs)

		for _, id := range delayedIDs {
			output := delayedOutputs[SiacoinOutputID(id)]
			leaves = append(leaves, crypto.HashObject(output))
		}
	}

	return crypto.MerkleRoot(leaves)
}

// BlockAtHeight returns the block on the current path with the given height.
func (s *State) BlockAtHeight(height BlockHeight) (b Block, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockAtHeight(height)
}

// Block returns the block associated with the given id.
func (s *State) Block(id BlockID) (b Block, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.blockMap[id]
	if !exists {
		return
	}
	b = node.block
	return
}

func (s *State) BlockOutputDiffs(id BlockID) (scods []SiacoinOutputDiff, err error) {
	node, exists := s.blockMap[id]
	if !exists {
		err = errors.New("requested an unknown block")
		return
	}
	if !node.diffsGenerated {
		err = errors.New("diffs have not been generated for the requested block.")
		return
	}
	scods = node.siacoinOutputDiffs
	return
}

// BlocksSince returns a set of output diffs representing how the state
// has changed since block 'id'. OutputDiffsSince will flip the `new` value for
// diffs that got reversed.
func (s *State) BlocksSince(id BlockID) (removedBlocks, addedBlocks []BlockID, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.blockMap[id]
	if !exists {
		err = errors.New("block is unknown")
		return
	}

	// Get all the IDs from going backwards to the blockchain.
	path := s.backtrackToBlockchain(node)
	for _, node := range path[1:] {
		removedBlocks = append(removedBlocks, node.block.ID())
	}

	// Get all the IDs going forward from the common parent.
	for height := path[0].height; ; height++ {
		if _, exists := s.currentPath[height]; !exists {
			break
		}

		node := s.blockMap[s.currentPath[height]]
		addedBlocks = append(addedBlocks, node.block.ID())
	}

	return
}

// Contract returns a the contract associated with the input id, and whether
// the contract exists.
func (s *State) FileContract(id FileContractID) (fc FileContract, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fc, exists = s.fileContracts[id]
	return
}

// CurrentBlock returns the highest block on the tallest fork.
func (s *State) CurrentBlock() Block {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().block
}

// CurrentTarget returns the target of the next block that needs to be
// submitted to the state.
func (s *State) CurrentTarget() Target {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().target
}

// EarliestLegalTimestamp returns the earliest legal timestamp of the next
// block - earlier timestamps will render the block invalid.
func (s *State) EarliestTimestamp() Timestamp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().earliestChildTimestamp()
}

// State.Height() returns the height of the longest fork.
func (s *State) Height() BlockHeight {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.height()
}

// HeightOfBlock returns the height of the block with id `bid`.
func (s *State) HeightOfBlock(bid BlockID) (height BlockHeight, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bn, exists := s.blockMap[bid]
	if !exists {
		return
	}
	height = bn.height
	return
}

// Output returns the output associated with an OutputID, returning an error if
// the output is not found.
func (s *State) SiacoinOutput(id SiacoinOutputID) (output SiacoinOutput, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.output(id)
}

func (s *State) SiafundOutput(id SiafundOutputID) (output SiafundOutput, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	output, exists = s.siafundOutputs[id]
	return
}

// Sorted UtxoSet returns all of the unspent transaction outputs sorted
// according to the numerical value of their id.
func (s *State) SortedUtxoSet() []SiacoinOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedUscoSet()
}

func (s *State) StorageProofSegment(fcid FileContractID) (index uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storageProofSegment(fcid)
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) StateHash() crypto.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Hash()
}

func (s *State) ValidTransaction(t Transaction) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validTransaction(t)
}

// ValidTransactionComponents checks that a transaction follows basic rules,
// such as the storage proof rules, and it checks that all of the signatures
// are valid, but it does not check that all of the inputs, storage proofs, and
// terminations act on existing outputs and contracts. This function is
// primarily for the transaction pool, which has access to unconfirmed
// transactions. ValidTransactionComponents will not return an error simply
// because there are missing inputs. ValidTransactionComponenets will return an
// error if the state height is not sufficient to fulfill all of the
// requirements of the transaction.
func (s *State) ValidTransactionComponents(t Transaction) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err = t.FollowsStorageProofRules()
	if err != nil {
		return
	}
	err = s.validFileContracts(t)
	if err != nil {
		return
	}
	err = s.validStorageProofs(t)
	if err != nil {
		return
	}
	err = s.validSignatures(t)
	if err != nil {
		return
	}

	return
}

func (s *State) ValidUnlockConditions(uc UnlockConditions, uh UnlockHash) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validUnlockConditions(uc, uh)
}
