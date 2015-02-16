package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

var (
	ErrMissingSignatures = errors.New("transaction has inputs with missing signatures")
)

// Each input has a list of public keys and a required number of signatures.
// inputSignatures keeps track of which public keys have been used and how many
// more signatures are needed.
type inputSignatures struct {
	remainingSignatures uint64
	possibleKeys        []SiaPublicKey
	usedKeys            map[uint64]struct{}
	index               int
}

// sortedUnique checks that 'elems' is sorted, contains no repeats, and that no
// element is an invalid index of 'elems'.
func sortedUnique(elems []uint64) bool {
	if len(elems) == 0 {
		return true
	}

	biggest := elems[0]
	for _, elem := range elems[1:] {
		if elem <= biggest {
			return false
		}
	}
	if biggest >= uint64(len(elems)) {
		return false
	}
	return true
}

// validCoveredFields makes sure that all covered fields objects in the
// signatures follow the rules. This means that if 'WholeTransaction' is set to
// true, all fields except for 'Signatures' must be empty. All fields must be
// sorted numerically, and there can be no repeats.
func (t Transaction) validCoveredFields() error {
	for _, sig := range t.Signatures {
		// convenience variables
		cf := sig.CoveredFields
		fields := [][]uint64{
			cf.SiacoinInputs,
			cf.MinerFees,
			cf.FileContracts,
			cf.FileContractTerminations,
			cf.StorageProofs,
			cf.SiafundInputs,
			cf.SiafundOutputs,
			cf.ArbitraryData,
		}

		// Check that all fields are empty if 'WholeTransaction' is set.
		if cf.WholeTransaction {
			for _, field := range fields {
				if len(field) != 0 {
					return errors.New("whole transaction flag is set, but not all fields besides signatures are empty")
				}
			}
		}

		// Check that all fields are sorted, and without repeat values, and
		// that all elements point to objects that exists within the
		// transaction.
		for _, field := range fields {
			if !sortedUnique(field) {
				return errors.New("field does not satisfy 'sorted and unique' requirement")
			}
		}
	}

	return nil
}

// validSignatures checks the validaty of all signatures in a transaction.
func (s *State) validSignatures(t Transaction) error {
	// Check that all covered fields objects follow the rules.
	err := t.validCoveredFields()
	if err != nil {
		return err
	}

	// Create the inputSignatures object for each input.
	sigMap := make(map[crypto.Hash]*inputSignatures)
	for i, input := range t.SiacoinInputs {
		id := crypto.Hash(input.ParentID)
		_, exists := sigMap[id]
		if exists {
			return errors.New("siacoin output spent twice in the same transaction.")
		}

		sigMap[id] = &inputSignatures{
			remainingSignatures: input.UnlockConditions.NumSignatures,
			possibleKeys:        input.UnlockConditions.PublicKeys,
			index:               i,
		}
	}
	for i, termination := range t.FileContractTerminations {
		id := crypto.Hash(termination.ParentID)
		_, exists := sigMap[id]
		if exists {
			return errors.New("file contract terminated twice in the same transaction.")
		}

		sigMap[id] = &inputSignatures{
			remainingSignatures: termination.TerminationConditions.NumSignatures,
			possibleKeys:        termination.TerminationConditions.PublicKeys,
			index:               i,
		}
	}
	for i, input := range t.SiafundInputs {
		id := crypto.Hash(input.ParentID)
		_, exists := sigMap[id]
		if exists {
			return errors.New("siafund output spent twice in the same transaction.")
		}

		sigMap[id] = &inputSignatures{
			remainingSignatures: input.UnlockConditions.NumSignatures,
			possibleKeys:        input.UnlockConditions.PublicKeys,
			index:               i,
		}
	}

	// Check all of the signatures for validity.
	for i, sig := range t.Signatures {
		// check that sig corresponds to an entry in sigMap
		inSig, exists := sigMap[crypto.Hash(sig.ParentID)]
		if !exists || inSig.remainingSignatures == 0 {
			return errors.New("frivolous signature in transaction")
		}
		// check that sig's key hasn't already been used
		_, exists = inSig.usedKeys[sig.PublicKeyIndex]
		if exists {
			return errors.New("one public key was used twice while signing an input")
		}
		// Check that the timelock has expired.
		if sig.Timelock > s.height() {
			return errors.New("signature used before timelock expiration")
		}

		// Check that the signature verifies. Multiple signature schemes are
		// supported.
		publicKey := inSig.possibleKeys[sig.PublicKeyIndex]
		switch publicKey.Algorithm {
		case SignatureEntropy:
			return crypto.ErrInvalidSignature

		case SignatureEd25519:
			// Decode the public key and signature.
			var edPK crypto.PublicKey
			err := encoding.Unmarshal([]byte(publicKey.Key), &edPK)
			if err != nil {
				return err
			}
			var edSig [crypto.SignatureSize]byte
			err = encoding.Unmarshal([]byte(sig.Signature), &edSig)
			if err != nil {
				return err
			}
			cryptoSig := crypto.Signature(&edSig)

			sigHash := t.SigHash(i)
			err = crypto.VerifyHash(sigHash, edPK, cryptoSig)
			if err != nil {
				return err
			}

		default:
			// If we don't recognize the identifier, assume that the signature
			// is valid. This allows more signature types to be added via soft
			// forking.
		}

		inSig.remainingSignatures--
	}

	// Check that all inputs have been sufficiently signed.
	for _, reqSigs := range sigMap {
		if reqSigs.remainingSignatures != 0 {
			return ErrMissingSignatures
		}
	}

	return nil
}
