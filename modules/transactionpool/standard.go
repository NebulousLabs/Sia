package transactionpool

import (
	"errors"

	"gitlab.com/NebulousLabs/Sia/encoding"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"
)

// standard.go adds extra rules to transactions which help preserve network
// health and provides flexibility for future soft forks and tweaks to the
// network.
//
// Rule: Transaction size is limited
//		There is a DoS vector where large transactions can both contain many
//		signatures, and have each signature's CoveredFields object cover a
//		unique but large portion of the transaction. A 1mb transaction could
//		force a verifier to hash very large volumes of data, which takes a long
//		time on nonspecialized hardware.
//
// Rule: Foreign signature algorithms are rejected.
//		There are plans to add newer, faster signature algorithms to Sia as the
//		project matures and the need for increased verification speed grows.
//		Foreign signatures are allowed into the blockchain, where they are
//		accepted as valid. Hoewver, if there has been a soft-fork, the foreign
//		signatures might actually be invalid. This rule protects legacy miners
//		from including potentially invalid transactions in their blocks.
//
// Rule: The types of allowed arbitrary data are limited
//		The arbitrary data field can be used to orchestrate soft-forks to Sia
//		that add features. Legacy miners are at risk of creating invalid blocks
//		if they include arbitrary data which has meanings that the legacy miner
//		doesn't understand.
//
// Rule: The transaction set size is limited.
//		A group of dependent transactions cannot exceed 100kb to limit how
//		quickly the transaction pool can be filled with new transactions.

// checkUnlockConditions looks at the UnlockConditions and verifies that all
// public keys are recognized. Unrecognized public keys are automatically
// accepted as valid by the consnensus set, but rejected by the transaction
// pool. This allows new types of keys to be added via a softfork without
// alienating all of the older nodes.
func checkUnlockConditions(uc types.UnlockConditions) error {
	for _, pk := range uc.PublicKeys {
		if pk.Algorithm != types.SignatureEntropy &&
			pk.Algorithm != types.SignatureEd25519 {
			return errors.New("unrecognized key type in transaction")
		}
	}

	return nil
}

// isStandardTransaction enforces extra rules such as a transaction size limit.
// These rules can be altered without disrupting consensus.
//
// The size of the transaction is returned so that the transaction does not need
// to be encoded multiple times.
func isStandardTransaction(t types.Transaction) (uint64, error) {
	// Check that the size of the transaction does not exceed the standard
	// established in Standard.md. Larger transactions are a DOS vector,
	// because someone can fill a large transaction with a bunch of signatures
	// that require hashing the entire transaction. Several hundred megabytes
	// of hashing can be required of a verifier. Enforcing this rule makes it
	// more difficult for attackers to exploid this DOS vector, though a miner
	// with sufficient power could still create unfriendly blocks.
	tlen := len(encoding.Marshal(t))
	if tlen > modules.TransactionSizeLimit {
		return 0, modules.ErrLargeTransaction
	}

	// Check that all public keys are of a recognized type. Need to check all
	// of the UnlockConditions, which currently can appear in 3 separate fields
	// of the transaction. Unrecognized types are ignored because a softfork
	// may make certain unrecognized signatures invalid, and this node cannot
	// tell which signatures are the invalid ones.
	for _, sci := range t.SiacoinInputs {
		err := checkUnlockConditions(sci.UnlockConditions)
		if err != nil {
			return 0, err
		}
	}
	for _, fcr := range t.FileContractRevisions {
		err := checkUnlockConditions(fcr.UnlockConditions)
		if err != nil {
			return 0, err
		}
	}
	for _, sfi := range t.SiafundInputs {
		err := checkUnlockConditions(sfi.UnlockConditions)
		if err != nil {
			return 0, err
		}
	}

	// Check that all arbitrary data is prefixed using the recognized set of
	// prefixes. The allowed prefixes include a 'NonSia' prefix for truly
	// arbitrary data. Blocking all other prefixes allows arbitrary data to be
	// used to orchestrate more complicated soft forks in the future without
	// putting older nodes at risk of violating the new rules.
	var prefix types.Specifier
	for _, arb := range t.ArbitraryData {
		// Check for a whilelisted prefix.
		copy(prefix[:], arb)
		if prefix == modules.PrefixHostAnnouncement ||
			prefix == modules.PrefixNonSia {
			continue
		}

		return 0, modules.ErrInvalidArbPrefix
	}
	return uint64(tlen), nil
}

// isStandardTransactionSet checks that all transacitons of a set follow the
// IsStandard guidelines, and that the set as a whole follows the guidelines as
// well.
//
// The size of the transaction set is returned so that the encoding only needs
// to happen once.
func isStandardTransactionSet(ts []types.Transaction) (uint64, error) {
	// Check that each transaction is acceptable, while also making sure that
	// the size of the whole set is legal.
	var totalSize uint64
	for i := range ts {
		tSize, err := isStandardTransaction(ts[i])
		if err != nil {
			return 0, err
		}
		totalSize += tSize
		if totalSize > modules.TransactionSetSizeLimit {
			return 0, modules.ErrLargeTransactionSet
		}

	}
	return totalSize, nil
}
