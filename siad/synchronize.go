package siad

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

const (
	MaxCatchUpBlocks = 100
)

// SendBlocks sends all known blocks from the given height forward from the
// longest known fork.
func (s *State) SendBlocks(knownBlocks [32]BlockID, blocks *[]Block) error {
	// Find the most recent block that is in our current path.
	found := false
	var closestHeight BlockHeight
	for i := range knownBlocks {
		// See if we know which block it is, get the node to know the height.
		blockNode, exists := s.blockMap[knownBlocks[i]]
		if !exists {
			continue
		}

		// See if the known block is in the current path, and if it is see if
		// its height is greater than the closest yet known height.
		id, exists := s.currentPath[blockNode.Height]
		if exists && id == knownBlocks[i] {
			found = true
			if closestHeight < blockNode.Height {
				closestHeight = blockNode.Height
			}
		}
	}

	// See that a match was actually found.
	if !found {
		return errors.New("no matching block found during SendBlocks")
	}

	// Build an array of blocks.
	tallest := closestHeight + MaxCatchUpBlocks
	if tallest > s.Height() {
		tallest = s.Height()
	}

	for i := closestHeight; i <= tallest; i++ {
		b, err := s.BlockAtHeight(i)
		if err != nil {
			panic(err)
		}
		*blocks = append(*blocks, b)
	}

	return nil
}

func (s *State) CatchUp(peer network.NetAddress) (err error) {
	var knownBlocks [32]BlockID
	for i := BlockHeight(0); i < 12; i++ {
		// Prevent underflows
		if i > s.Height() {
			break
		}

		knownBlocks[i] = s.currentPath[s.Height()-i]
	}

	backtrace := BlockHeight(10)
	for i := 12; i < 31; i++ {
		backtrace = BlockHeight(float64(backtrace) * 1.75)
		// Prevent underflows
		if backtrace > s.Height() {
			break
		}

		knownBlocks[i] = s.currentPath[s.Height()-backtrace]
	}

	knownBlocks[31] = s.currentPath[0]
	prevHeight := s.Height()

	// Dirty, but we can't make network calls while the state is locked - can
	// cause deadlock.
	var blocks []Block
	s.Unlock()
	err = peer.RPC("SendBlocks", knownBlocks, &blocks)
	s.Lock()
	if err != nil {
		return err
	}

	for i := range blocks {
		if err = s.AcceptBlock(blocks[i]); err != nil {
			if err != BlockKnownErr && err != FutureBlockErr {
				// Return if there's an error, but don't return for benign
				// errors: BlockKnownErr and FutureBlockErr are both benign.
				return err
			}
		}
	}

	// recurse until the height stops increasing
	if prevHeight != s.Height() {
		return s.CatchUp(peer)
	}

	return nil
}
