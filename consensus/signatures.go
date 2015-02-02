package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
)

// TODO: when testing the covered fields stuff, antagonistically try to cause
// seg faults by throwing covered fields objects at the state which point to
// nonexistent objects in the transaction.

// Each input has a list of public keys and a required number of signatures.
// InputSignatures keeps track of which public keys have been used and how many
// more signatures are needed.
type InputSignatures struct {
	RemainingSignatures uint64
	PossibleKeys        []PublicKey
	UsedKeys            map[uint64]struct{}
	Index               int
}

// validStorageProofs checks that a transaction follows the limitations placed
// on transactions with storage proofs.
func (t Transaction) validStorageProofs() bool {
	if len(t.StorageProofs) == 0 {
		return true
	}

	if len(t.Outputs) != 0 {
		return false
	}
	if len(t.FileContracts) != 0 {
		return false
	}
	if len(t.SiafundOutputs) != 0 {
		return false
	}

	return true
}

// sortedUnique checks that all of the elements in a []unit64 are sorted and
// without repeates, and also checks that the largest element is less than or
// equal to the biggest allowed element.
func sortedUnique(elems []uint64, biggestAllowed int) (err error) {
	biggest := elems[0]
	for _, elem := range elems[1:] {
		if elem <= biggest {
			err = errors.New("covered fields sorting violation")
			return
		}
	}
	if int(biggest) > biggestAllowed {
		err = errors.New("covered fields indexing violation")
		return
	}
	return
}

// validCoveredFields makes sure that all covered fields objects in the
// signatures follow the rules. This means that if `WholeTransaction` is set to
// true, all fields except for `Signatures` must be empty. All fields must be
// sorted numerically, and there can be no repeats.
func (t Transaction) validCoveredFields() (err error) {
	for _, sig := range t.Signatures {
		// Check that all fields are empty if `WholeTransaction` is set.
		cf := sig.CoveredFields
		if cf.WholeTransaction {
			if len(cf.Inputs) != 0 ||
				len(cf.MinerFees) != 0 ||
				len(cf.Outputs) != 0 ||
				len(cf.FileContracts) != 0 ||
				len(cf.StorageProofs) != 0 ||
				len(cf.SiafundInputs) != 0 ||
				len(cf.SiafundOutputs) != 0 ||
				len(cf.ArbitraryData) != 0 {
				err = errors.New("whole transaction is set but not all fields besides signatures are empty")
				return
			}
		}

		// Check that all fields are sorted, and without repeat values, and
		// that all elements point to objects that exists within the
		// transaction.
		err = sortedUnique(cf.Inputs, len(cf.Inputs)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.MinerFees, len(cf.MinerFees)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.Outputs, len(cf.Outputs)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.FileContracts, len(cf.FileContracts)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.StorageProofs, len(cf.StorageProofs)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.SiafundInputs, len(cf.SiafundInputs)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.SiafundOutputs, len(cf.SiafundOutputs)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.ArbitraryData, len(cf.ArbitraryData)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.Signatures, len(cf.Signatures)-1)
		if err != nil {
			return
		}
	}

	return
}

func (s *State) validSignatures(t Transaction) (err error) {
	// Check that all covered fields objects follow the rules.
	err = t.validCoveredFields()
	if err != nil {
		return
	}

	// Create the InputSignatures object for each input.
	sigMap := make(map[OutputID]*InputSignatures)
	for i, input := range t.Inputs {
		_, exists := sigMap[input.OutputID]
		if exists {
			return errors.New("output spent twice in the same transaction.")
		}
		inSig := &InputSignatures{
			RemainingSignatures: input.SpendConditions.NumSignatures,
			PossibleKeys:        input.SpendConditions.PublicKeys,
			Index:               i,
		}
		sigMap[input.OutputID] = inSig
	}

	// Check all of the signatures for validity.
	for i, sig := range t.Signatures {
		// Check that each signature signs a unique pubkey where
		// RemainingSignatures > 0.
		if sigMap[sig.InputID].RemainingSignatures == 0 {
			return errors.New("frivolous signature in transaction")
		}
		_, exists := sigMap[sig.InputID].UsedKeys[sig.PublicKeyIndex]
		if exists {
			return errors.New("one public key was used twice while signing an input")
		}
		if sig.TimeLock > s.height() {
			return errors.New("signature used before timelock expiration")
		}

		// Check that the signature matches the public key + data.
		sigHash := t.SigHash(i)
		if !crypto.VerifyBytes(sigHash[:], sigMap[sig.InputID].PossibleKeys[sig.PublicKeyIndex], sig.Signature) {
			return errors.New("signature is invalid")
		}

		// Subtract the number of signatures remaining for this input.
		sigMap[sig.InputID].RemainingSignatures -= 1
	}

	// Check that all inputs have been sufficiently signed.
	for _, reqSigs := range sigMap {
		if reqSigs.RemainingSignatures != 0 {
			return errors.New("some inputs are missing signatures")
		}
	}

	return nil
}

func (s *State) ValidSignatures(t Transaction) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validSignatures(t)
}
