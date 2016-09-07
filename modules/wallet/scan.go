package wallet

import (
	"fmt"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	numInitialKeys = 1e6
	maxScanKeys    = 100e6
)

var errMaxKeys = fmt.Errorf("refused to generate more than %v keys from seed", maxScanKeys)

// A seedScanner scans the blockchain for addresses that belong to a given
// seed.
type seedScanner struct {
	seed             modules.Seed
	keys             map[types.UnlockHash]uint64 // map address to seed index
	largestIndexSeen uint64                      // largest index that has appeared in the blockchain
}

func (s *seedScanner) isSeedAddress(uh types.UnlockHash) bool {
	_, exists := s.keys[uh]
	return exists
}

// generateKeys generates n additional keys from the seedScanner's seed.
func (s *seedScanner) generateKeys(n uint64) {
	initialProgress := uint64(len(s.keys))
	for i, k := range generateKeys(s.seed, initialProgress, n) {
		s.keys[k.UnlockConditions.UnlockHash()] = initialProgress + uint64(i)
	}
}

func (s *seedScanner) ProcessConsensusChange(cc modules.ConsensusChange) {
	var addrs []types.UnlockHash
	for _, diff := range cc.SiacoinOutputDiffs {
		addrs = append(addrs, diff.SiacoinOutput.UnlockHash)
	}
	for _, diff := range cc.SiafundOutputDiffs {
		addrs = append(addrs, diff.SiafundOutput.UnlockHash)
	}

	for _, block := range cc.AppliedBlocks {
		for _, mp := range block.MinerPayouts {
			addrs = append(addrs, mp.UnlockHash)
		}
		for _, txn := range block.Transactions {
			for _, sci := range txn.SiacoinInputs {
				addrs = append(addrs, sci.UnlockConditions.UnlockHash())
			}
			for _, sco := range txn.SiacoinOutputs {
				addrs = append(addrs, sco.UnlockHash)
			}
			for _, sfi := range txn.SiafundInputs {
				addrs = append(addrs, sfi.UnlockConditions.UnlockHash())
			}
			for _, sfo := range txn.SiafundOutputs {
				addrs = append(addrs, sfo.UnlockHash)
			}
		}
	}

	// update largestIndexSeen
	for _, addr := range addrs {
		index, exists := s.keys[addr]
		if exists && index > s.largestIndexSeen {
			s.largestIndexSeen = index
		}
	}
}

// scan subscribes d to cs and scans the blockchain for addresses that belong
// to d's seed. If scan returns errMaximumKeys, additional keys may need to be
// generated to find all the addresses.
func (s *seedScanner) scan(cs modules.ConsensusSet) error {
	// generate a bunch of keys and scan the blockchain looking for them. If
	// none of the keys are found, we are done; otherwise, generate more keys
	// and try again (bounded by a sane default).
	//
	// NOTE: since scanning is very slow, we aim to only scan once, which
	// means generating many keys.
	var numKeys uint64 = numInitialKeys
	for len(s.keys) < maxScanKeys {
		s.generateKeys(numKeys)
		if err := cs.ConsensusSetSubscribe(s, modules.ConsensusChangeBeginning); err != nil {
			return err
		}
		if s.largestIndexSeen < uint64(len(s.keys))/2 {
			cs.Unsubscribe(s)
			return nil
		}
		// double number of keys generated each iteration, capping so that we
		// do not exceed maxScanKeys
		numKeys *= 2
		if numKeys > maxScanKeys-uint64(len(s.keys)) {
			numKeys = maxScanKeys - uint64(len(s.keys))
		}
	}
	cs.Unsubscribe(s)
	return errMaxKeys
}

func newSeedScanner(seed modules.Seed) *seedScanner {
	return &seedScanner{
		seed: seed,
		keys: make(map[types.UnlockHash]uint64),
	}
}
