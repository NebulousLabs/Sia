package consensus

import (
	"errors"
	"math/big"
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
)

// Contains basic information about the state, but does not go into depth.
type StateInfo struct {
	CurrentBlock BlockID
	Height       BlockHeight
	Target       Target
}

// BlockAtHeight() returns the block from the current history at the
// input height.
func (s *State) blockAtHeight(height BlockHeight) (b Block, exists bool) {
	bn, exists := s.blockMap[s.currentPath[height]]
	if !exists {
		return
	}
	b = bn.block
	return
}

// currentBlockNode returns the node of the most recent block in the
// longest fork.
func (s *State) currentBlockNode() *blockNode {
	return s.blockMap[s.currentBlockID]
}

// CurrentBlockWeight() returns the weight of the current block in the
// heaviest fork.
func (s *State) currentBlockWeight() *big.Rat {
	return s.currentBlockNode().target.Inverse()
}

// depth returns the depth of the current block of the state.
func (s *State) depth() Target {
	return s.currentBlockNode().depth
}

// height returns the current height of the state.
func (s *State) height() BlockHeight {
	return s.blockMap[s.currentBlockID].height
}

// State.Output returns the Output associated with the id provided for input,
// but only if the output is a part of the utxo set.
func (s *State) output(id SiacoinOutputID) (sco SiacoinOutput, exists bool) {
	sco, exists = s.siacoinOutputs[id]
	return
}

// Sorted UscoSet returns all of the unspent transaction outputs sorted
// according to the numerical value of their id.
func (s *State) sortedUscoSet() (sortedOutputs []SiacoinOutput) {
	// Get all of the outputs in string form and sort the strings.
	var unspentOutputStrings []string
	for outputID := range s.siacoinOutputs {
		unspentOutputStrings = append(unspentOutputStrings, string(outputID[:]))
	}
	sort.Strings(unspentOutputStrings)

	// Get the outputs in order according to their sorted string form.
	for _, utxoString := range unspentOutputStrings {
		var outputID SiacoinOutputID
		copy(outputID[:], utxoString)
		output, _ := s.output(outputID)
		sortedOutputs = append(sortedOutputs, output)
	}
	return
}

// Sorted UsfoSet returns all of the unspent siafund outputs sorted according
// to the numerical value of their id.
func (s *State) sortedUsfoSet() (sortedOutputs []SiafundOutput) {
	// Get all of the outputs in string form and sort the strings.
	var idStrings []string
	for outputID := range s.siafundOutputs {
		idStrings = append(idStrings, string(outputID[:]))
	}
	sort.Strings(idStrings)

	// Get the outputs in order according to their sorted string form.
	for _, idString := range idStrings {
		var outputID SiafundOutputID
		copy(outputID[:], idString)

		// Sanity check - the output should exist.
		output, exists := s.siafundOutputs[outputID]
		if DEBUG {
			if !exists {
				panic("output doesn't exist")
			}
		}

		sortedOutputs = append(sortedOutputs, output)
	}
	return
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) stateHash() crypto.Hash {
	// Items of interest:
	// 1.	genesis block
	// 2.	current block id
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

	// Get the set of siacoin outputs in sorted order and add them.
	sortedUscos := s.sortedUscoSet()
	for _, output := range sortedUscos {
		leaves = append(leaves, crypto.HashObject(output))
	}

	// Sort the open contracts by the string value of their ID.
	var openContractStrings []string
	for contractID := range s.fileContracts {
		openContractStrings = append(openContractStrings, string(contractID[:]))
	}
	sort.Strings(openContractStrings)

	// Add the open contracts in sorted order.
	for _, stringContractID := range openContractStrings {
		var contractID FileContractID
		copy(contractID[:], stringContractID)
		leaves = append(leaves, crypto.HashObject(s.fileContracts[contractID]))
	}

	// Get the set of siafund outputs in sorted order and add them.
	sortedUsfos := s.sortedUsfoSet()
	for _, output := range sortedUsfos {
		leaves = append(leaves, crypto.HashObject(output))
	}

	// Get the set of delayed siacoin outputs, sorted by maturity height then
	// sorted by id and add them.
	for i := BlockHeight(0); i <= s.height(); i++ {
		delayedOutputs := s.delayedSiacoinOutputs[i]
		var delayedStrings []string
		for id := range delayedOutputs {
			delayedStrings = append(delayedStrings, string(id[:]))
		}
		sort.Strings(delayedStrings)

		for _, delayedString := range delayedStrings {
			var id SiacoinOutputID
			copy(id[:], delayedString)
			leaves = append(leaves, crypto.HashObject(delayedOutputs[id]))
		}
	}

	return crypto.MerkleRoot(leaves)
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

// BlockAtHeight returns the block in the current fork found at `height`.
func (s *State) BlockAtHeight(height BlockHeight) (b Block, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bn, exists := s.blockMap[s.currentPath[height]]
	if !exists {
		return
	}
	b = bn.block
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

	// Get all the ids from going backwards to the blockchain.
	reversedNodes := s.backtrackToBlockchain(node)
	height := reversedNodes[len(reversedNodes)-1].height
	// Eliminate the last node, which is the pivot node, whose diffs are already
	// known.
	reversedNodes = reversedNodes[:len(reversedNodes)-1]
	for _, reversedNode := range reversedNodes {
		removedBlocks = append(removedBlocks, reversedNode.block.ID())
	}

	// Get all the ids going forward from the pivot node.
	for _, exists := s.currentPath[height]; exists; height++ {
		node := s.blockMap[s.currentPath[height]]
		addedBlocks = append(addedBlocks, node.block.ID())
		_, exists = s.currentPath[height+1]
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
	return s.stateHash()
}

func (s *State) ValidTransaction(t Transaction) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validTransaction(t)
}

// ValidTransactionComponenets checks that a transaction follows basic rules,
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
