package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/types"
)

// checkDelayedSiacoinOutputMaps checks that the delayed siacoin output maps
// have the right number of maps at the right heights.
func (cs *State) checkDelayedSiacoinOutputMaps() error {
	expected := 0
	for i := cs.height() + 1; i <= cs.height()+types.MaturityDelay; i++ {
		if !(i > types.MaturityDelay) {
			continue
		}
		_, exists := cs.delayedSiacoinOutputs[i]
		if !exists {
			return errors.New("delayed siacoin outputs are in an inconsistent state")
		}
		expected++
	}
	if len(cs.delayedSiacoinOutputs) != expected {
		return errors.New("delayed siacoin outputs has too many maps")
	}

	return nil
}
