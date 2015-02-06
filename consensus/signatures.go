package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

// TODO: when testing the covered fields stuff, antagonistically try to cause
// seg faults by throwing covered fields objects at the state which point to
// nonexistent objects in the transaction.

var (
	InvalidSignatureErr  = errors.New("signature is invalid")
	MissingSignaturesErr = errors.New("transaction has inputs with missing signatures")
)

// Each input has a list of public keys and a required number of signatures.
// InputSignatures keeps track of which public keys have been used and how many
// more signatures are needed.
type InputSignatures struct {
	RemainingSignatures uint64
	PossibleKeys        []SiaPublicKey
	UsedKeys            map[uint64]struct{}
	Index               int
}

// sortedUnique checks that all of the elements in a []unit64 are sorted and
// without repeates, and also checks that the largest element is less than or
// equal to the biggest allowed element.
func sortedUnique(elems []uint64, biggestAllowed int) (err error) {
	if len(elems) == 0 {
		return
	}

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
			if len(cf.SiacoinInputs) != 0 ||
				len(cf.MinerFees) != 0 ||
				len(cf.SiacoinOutputs) != 0 ||
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
		err = sortedUnique(cf.SiacoinInputs, len(cf.SiacoinInputs)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.MinerFees, len(cf.MinerFees)-1)
		if err != nil {
			return
		}
		err = sortedUnique(cf.SiacoinOutputs, len(cf.SiacoinOutputs)-1)
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

// validSignatures takes a transaction and returns an error if the signatures
// are not all valid.
func (s *State) validSignatures(t Transaction) (err error) {
	// Check that all covered fields objects follow the rules.
	err = t.validCoveredFields()
	if err != nil {
		return
	}

	// Create the InputSignatures object for each input.
	sigMap := make(map[OutputID]*InputSignatures)
	for i, input := range t.SiacoinInputs {
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

		// Check that the timelock has expired.
		if sig.TimeLock > s.height() {
			return errors.New("signature used before timelock expiration")
		}

		// Check that the signature verifies. Sia is built to support multiple
		// types of signature algorithms, this is handled by the switch
		// statement.
		publicKey := sigMap[sig.InputID].PossibleKeys[sig.PublicKeyIndex]
		switch publicKey.Algorithm {
		case ED25519Identifier:
			// Decode the public key and signature.
			var decodedPK crypto.PublicKey
			err := encoding.Unmarshal(publicKey.Key, &decodedPK)
			if err != nil {
				return err
			}
			var decodedSig crypto.Signature
			err = encoding.Unmarshal(sig.Signature, &decodedSig)
			if err != nil {
				return err
			}

			sigHash := t.SigHash(i)
			if !crypto.VerifyBytes(sigHash, decodedPK, decodedSig) {
				return InvalidSignatureErr
			}
		default:
			// If we don't recognize the identifier, assume that the signature
			// is valid; do nothing. This allows more signature types to be
			// added through soft forking.
		}

		// Subtract the number of signatures remaining for this input.
		sigMap[sig.InputID].RemainingSignatures -= 1
	}

	// Check that all inputs have been sufficiently signed.
	for _, reqSigs := range sigMap {
		if reqSigs.RemainingSignatures != 0 {
			return MissingSignaturesErr
		}
	}

	return nil
}

// ValidSignatures takes a transaction and determines whether the transaction
// contains a legal set of signatures, including checking the timelocks against
// the current state height.
func (s *State) ValidSignatures(t Transaction) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validSignatures(t)
}
