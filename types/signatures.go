package types

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

var (
	// These Specifiers enumerate the types of signatures that are recognized
	// by this implementation. If a signature's type is unrecognized, the
	// signature is treated as valid. Signatures using the special "entropy"
	// type are always treated as invalid; see Consensus.md for more details.
	SignatureEntropy = Specifier{'e', 'n', 't', 'r', 'o', 'p', 'y'}
	SignatureEd25519 = Specifier{'e', 'd', '2', '5', '5', '1', '9'}

	ErrMissingSignatures = errors.New("transaction has inputs with missing signatures")

	ZeroUnlockHash = UnlockHash{0}
)

type (
	Signature string
)

// UnlockConditions are a set of conditions which must be met to execute
// certain actions, such as spending a SiacoinOutput or terminating a
// FileContract.
//
// The simplest requirement is that the block containing the UnlockConditions
// must have a height >= 'Timelock'.
//
// 'PublicKeys' specifies the set of keys that can be used to satisfy the
// UnlockConditions; of these, at least 'NumSignatures' unique keys must sign
// the transaction. The keys that do not need to use the same cryptographic
// algorithm.
//
// If 'NumSignatures' == 0, the UnlockConditions are effectively "anyone can
// unlock." If 'NumSignatures' > len('PublicKeys'), then the UnlockConditions
// cannot be fulfilled under any circumstances.
type UnlockConditions struct {
	Timelock      BlockHeight
	PublicKeys    []SiaPublicKey
	NumSignatures uint64
}

// A SiaPublicKey is a public key prefixed by a Specifier. The Specifier
// indicates the algorithm used for signing and verification. Unrecognized
// algorithms will always verify, which allows new algorithms to be added to
// the protocol via a soft-fork.
type SiaPublicKey struct {
	Algorithm Specifier
	Key       string
}

// A TransactionSignature is a signature that is included in the transaction.
// The signature should correspond to a public key in one of the
// UnlockConditions of the transaction. This key is specified first by
// 'ParentID', which specifies the UnlockConditions, and then
// 'PublicKeyIndex', which indicates the key in the UnlockConditions. There
// are three types that use UnlockConditions: SiacoinInputs, SiafundInputs,
// and FileContractTerminations. Each of these types also references a
// ParentID, and this is the hash that 'ParentID' must match. The 'Timelock'
// prevents the signature from being used until a certain height.
// 'CoveredFields' indicates which parts of the transaction are being signed;
// see CoveredFields.
type TransactionSignature struct {
	ParentID       crypto.Hash
	PublicKeyIndex uint64
	Timelock       BlockHeight
	CoveredFields  CoveredFields
	Signature      Signature
}

// CoveredFields indicates which fields in a transaction have been covered by
// the signature. (Note that the signature does not sign the fields
// themselves, but rather their combined hash; see SigHash.) Each slice
// corresponds to a slice in the Transaction type, indicating which indices of
// the slice have been signed. The indices must be valid, i.e. within the
// bounds of the slice. In addition, they must be sorted and unique.
//
// As a convenience, a signature of the entire transaction can be indicated by
// the 'WholeTransaction' field. If 'WholeTransaction' == true, all other
// fields must be empty (except for the Signatures field, since a signature
// cannot sign itself).
type CoveredFields struct {
	WholeTransaction         bool
	SiacoinInputs            []uint64
	SiacoinOutputs           []uint64
	FileContracts            []uint64
	FileContractTerminations []uint64
	StorageProofs            []uint64
	SiafundInputs            []uint64
	SiafundOutputs           []uint64
	MinerFees                []uint64
	ArbitraryData            []uint64
	Signatures               []uint64
}

// UnlockHash calculates the root hash of a Merkle tree of the
// UnlockConditions object. The leaves of this tree are formed by taking the
// hash of the timelock, the hash of the public keys (one leaf each), and the
// hash of the number of signatures. The keys are put in the middle because
// Timelock and NumSignatures are both low entropy fields; they can be
// protected by having random public keys next to them.
func (uc UnlockConditions) UnlockHash() UnlockHash {
	tree := crypto.NewTree()
	tree.PushObject(uc.Timelock)
	for i := range uc.PublicKeys {
		tree.PushObject(uc.PublicKeys[i])
	}
	tree.PushObject(uc.NumSignatures)
	return UnlockHash(tree.Root())
}

// SigHash returns the hash of the fields in a transaction covered by a given
// signature. See CoveredFields for more details.
func (t Transaction) SigHash(i int) crypto.Hash {
	cf := t.Signatures[i].CoveredFields
	var signedData []byte
	if cf.WholeTransaction {
		signedData = encoding.MarshalAll(
			t.SiacoinInputs,
			t.SiacoinOutputs,
			t.FileContracts,
			t.FileContractTerminations,
			t.StorageProofs,
			t.SiafundInputs,
			t.SiafundOutputs,
			t.MinerFees,
			t.ArbitraryData,
			t.Signatures[i].ParentID,
			t.Signatures[i].PublicKeyIndex,
			t.Signatures[i].Timelock,
		)
	} else {
		for _, input := range cf.SiacoinInputs {
			signedData = append(signedData, encoding.Marshal(t.SiacoinInputs[input])...)
		}
		for _, output := range cf.SiacoinOutputs {
			signedData = append(signedData, encoding.Marshal(t.SiacoinOutputs[output])...)
		}
		for _, contract := range cf.FileContracts {
			signedData = append(signedData, encoding.Marshal(t.FileContracts[contract])...)
		}
		for _, termination := range cf.FileContractTerminations {
			signedData = append(signedData, encoding.Marshal(t.FileContractTerminations[termination])...)
		}
		for _, storageProof := range cf.StorageProofs {
			signedData = append(signedData, encoding.Marshal(t.StorageProofs[storageProof])...)
		}
		for _, siafundInput := range cf.SiafundInputs {
			signedData = append(signedData, encoding.Marshal(t.SiafundInputs[siafundInput])...)
		}
		for _, siafundOutput := range cf.SiafundOutputs {
			signedData = append(signedData, encoding.Marshal(t.SiafundOutputs[siafundOutput])...)
		}
		for _, minerFee := range cf.MinerFees {
			signedData = append(signedData, encoding.Marshal(t.MinerFees[minerFee])...)
		}
		for _, arbData := range cf.ArbitraryData {
			signedData = append(signedData, encoding.Marshal(t.ArbitraryData[arbData])...)
		}
	}

	for _, sig := range cf.Signatures {
		signedData = append(signedData, encoding.Marshal(t.Signatures[sig])...)
	}

	return crypto.HashBytes(signedData)
}

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
// element is larger than or equal to 'max'.
func sortedUnique(elems []uint64, max int) bool {
	if len(elems) == 0 {
		return true
	}

	biggest := elems[0]
	for _, elem := range elems[1:] {
		if elem <= biggest {
			return false
		}
	}
	if biggest >= uint64(max) {
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
		fieldMaxs := []struct {
			field []uint64
			max   int
		}{
			{cf.SiacoinInputs, len(t.SiacoinInputs)},
			{cf.MinerFees, len(t.MinerFees)},
			{cf.FileContracts, len(t.FileContracts)},
			{cf.FileContractTerminations, len(t.FileContractTerminations)},
			{cf.StorageProofs, len(t.StorageProofs)},
			{cf.SiafundInputs, len(t.SiafundInputs)},
			{cf.SiafundOutputs, len(t.SiafundOutputs)},
			{cf.ArbitraryData, len(t.ArbitraryData)},
			{cf.Signatures, len(t.Signatures)},
		}

		// Check that all fields are empty if 'WholeTransaction' is set.
		if cf.WholeTransaction {
			// 'WholeTransaction' does not check signatures.
			for _, fieldMax := range fieldMaxs[:len(fieldMaxs)-1] {
				if len(fieldMax.field) != 0 {
					return errors.New("whole transaction flag is set, but not all fields besides signatures are empty")
				}
			}
		}

		// Check that all fields are sorted, and without repeat values, and
		// that all elements point to objects that exists within the
		// transaction.
		for _, fieldMax := range fieldMaxs {
			if !sortedUnique(fieldMax.field, fieldMax.max) {
				return errors.New("field does not satisfy 'sorted and unique' requirement")
			}
		}
	}

	return nil
}

// validSignatures checks the validaty of all signatures in a transaction.
func (t *Transaction) validSignatures(currentHeight BlockHeight) error {
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
			return errors.New("siacoin output spent twice in the same transaction")
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
			return errors.New("file contract terminated twice in the same transaction")
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
			return errors.New("siafund output spent twice in the same transaction")
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
		if sig.Timelock > currentHeight {
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
			cryptoSig := crypto.Signature(edSig)

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
