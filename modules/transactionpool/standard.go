package transactionpool

import (
	"errors"
	"strings"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// standard.go adds extra rules to transactions which help preserve network
// health and provides flexibility for future soft forks and tweaks to the
// network.
// Rule: Transaction size is limited
//   Purpose: Minimizes DOS vectors but maximizes future flexibility
// Rule: Foreign signature algorithms are ignored
//   Purpose: Prevent violating future rules where new signature algorithms
// Rule: The types of allowed arbitrary data are limited
//   Purpose: Leave room for more involved soft-forks in the future.

var (
	ErrLargeTransaction = errors.New("transaction is too large")
)

// checkUnlockConditions looks at the UnlockConditions and verifies that all
// public keys are recognized. Unrecognized public keys are automatically
// accpeted as valid by the state, but rejected by the transaction pool. This
// allows new types of keys to be added via a softfork without alienating all
// of the older nodes.
func (tp *TransactionPool) checkUnlockConditions(uc types.UnlockConditions) error {
	for _, pk := range uc.PublicKeys {
		if pk.Algorithm != types.SignatureEntropy &&
			pk.Algorithm != types.SignatureEd25519 {
			return errors.New("unrecognized key type in transaction")
		}
	}

	return nil
}

// IsStandardTransaction enforces extra rules such as a transaction size limit.
// These rules can be altered without disrupting consensus.
func (tp *TransactionPool) IsStandardTransaction(t types.Transaction) (err error) {
	// Check that the size of the transaction does not exceed the standard
	// established in Standard.md. Larger transactions are a DOS vector,
	// because someone can fill a large transaction with a bunch of signatures
	// that require hashing the entire transaction. Several hundred megabytes
	// of hashing can be required of a verifier. Enforcing this rule makes it
	// more difficult for attackers to exploid this DOS vector, though a miner
	// with sufficient power could still create unfriendly blocks.
	if len(encoding.Marshal(t)) > modules.TransactionSizeLimit {
		return ErrLargeTransaction
	}

	// Check that all public keys are of a recognized type. Need to check all
	// of the UnlockConditions, which currently can appear in 3 separate fields
	// of the transaction. Unrecognized types are ignored because a softfork
	// may make certain unrecognized signatures invalid, and this node cannot
	// tell which sigantures are the invalid ones.
	for _, sci := range t.SiacoinInputs {
		err = tp.checkUnlockConditions(sci.UnlockConditions)
		if err != nil {
			return
		}
	}
	for _, fcr := range t.FileContractRevisions {
		err = tp.checkUnlockConditions(fcr.UnlockConditions)
		if err != nil {
			return
		}
	}
	for _, sfi := range t.SiafundInputs {
		err = tp.checkUnlockConditions(sfi.UnlockConditions)
		if err != nil {
			return
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

		// COMPATv0.3.3.3 - deprecated whitelisted prefixes.
		strData := string(arb)
		if strings.HasPrefix(strData, modules.PrefixStrNonSia) {
			continue
		}

		return modules.ErrInvalidArbPrefix
	}

	return
}
