package consensus

// A list of blocks that were inverted and then applied in the most recent
// alteration of the state. If there are inverted blocks, InvertedBlocks[0]
// will be identical to StartingPoint.
type ConsensusChange struct {
	StartingPoint  BlockID
	InvertedBlocks []BlockID
	AppliedBlocks  []BlockID
}

// ConsensusSubscribe returns a channel that will receive a ConsensusChange
// notification each time that the consensus changes (from incoming blocks or
// invalidated blocks, etc.).
func (s *State) ConsensusSubscribe() (alert chan ConsensusChange) {
	alert = make(chan ConsensusChange)
	s.consensusSubscriptions = append(s.consensusSubscriptions, alert)
	return
}
