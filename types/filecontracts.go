package types

// filecontracts.go contains the basic structs and helper functions for file
// contracts.

import (
	"github.com/NebulousLabs/Sia/crypto"
)

type (
	// A FileContract is a public record of a storage agreement between a "host"
	// and a "renter." It mandates that a host must submit a storage proof to the
	// network, proving that they still possess the file they have agreed to
	// store.
	//
	// The party must submit the storage proof in a block that is between 'Start'
	// and 'Expiration'. Upon submitting the proof, the outputs for
	// 'ValidProofOutputs' are created. If the party does not submit a storage
	// proof by 'Expiration', then the outputs for 'MissedProofOutputs' are
	// created instead. The sum of 'MissedProofOutputs' must equal 'Payout', and
	// the sum of 'ValidProofOutputs' must equal 'Payout' plus the siafund fee.
	// This fee is sent to the siafund pool, which is a set of siacoins only
	// spendable by siafund owners.
	//
	// Under normal circumstances, the payout will be funded by both the host and
	// the renter, which gives the host incentive not to lose the file. The
	// 'ValidProofUnlockHash' will typically be spendable by host, and the
	// 'MissedProofUnlockHash' will either by spendable by the renter or by
	// nobody (the ZeroUnlockHash).
	//
	// A contract can be terminated early by submitting a FileContractTermination
	// whose UnlockConditions hash to 'TerminationHash'.
	FileContract struct {
		FileSize           uint64
		FileMerkleRoot     crypto.Hash
		Start              BlockHeight
		Expiration         BlockHeight
		Payout             Currency
		ValidProofOutputs  []SiacoinOutput
		MissedProofOutputs []SiacoinOutput
		TerminationHash    UnlockHash
	}

	// A FileContractTermination terminates a file contract. The ParentID
	// specifies the contract being terminated, and the TerminationConditions are
	// the conditions under which termination will be treated as valid. The hash
	// of the TerminationConditions must match the TerminationHash in the
	// contract. 'Payouts' is a set of SiacoinOutputs describing how the payout of
	// the contract is redistributed. It follows that the sum of these outputs
	// must equal the original payout. The outputs can have any Value and
	// UnlockHash, and do not need to match the ValidProofUnlockHash or
	// MissedProofUnlockHash of the original FileContract.
	FileContractTermination struct {
		ParentID              FileContractID
		TerminationConditions UnlockConditions
		Payouts               []SiacoinOutput
	}

	// A StorageProof fulfills a FileContract. The proof contains a specific
	// segment of the file, along with a set of hashes from the file's Merkle
	// tree. In combination, these can be used to prove that the segment came from
	// the file. To prevent abuse, the segment must be chosen randomly, so the ID
	// of block 'Start' - 1 is used as a seed value; see StorageProofSegment for
	// the exact implementation.
	//
	// A transaction with a StorageProof cannot have any SiacoinOutputs,
	// SiafundOutputs, or FileContracts. This is because a mundane reorg can
	// invalidate the proof, and with it the rest of the transaction.
	StorageProof struct {
		ParentID FileContractID
		Segment  [crypto.SegmentSize]byte
		HashSet  []crypto.Hash
	}
)

// FileContractTerminationPayoutID returns the ID of a file contract
// termination payout, given the index of the payout in the termination. The
// ID is calculated by hashing the concatenation of the
// FileContractTerminationPayout Specifier, the ID of the file contract being
// terminated, and the payout index.
func (fcid FileContractID) FileContractTerminationPayoutID(i int) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		SpecifierFileContractTerminationPayout,
		fcid,
		i,
	))
}

// StorageProofOutputID returns the ID of an output created by a file
// contract, given the status of the storage proof. The ID is calculating by
// hashing the concatenation of the StorageProofOutput Specifier, the ID of
// the file contract that the proof is for, a boolean indicating whether the
// proof was valid (true) or missed (false), and the index of the output
// within the file contract.
func (fcid FileContractID) StorageProofOutputID(proofValid bool, i int) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		SpecifierStorageProofOutput,
		fcid,
		proofValid,
		i,
	))
}

// Tax returns the amount of Currency that will be taxed from fc.
func (fc FileContract) Tax() Currency {
	return fc.Payout.MulFloat(SiafundPortion).RoundDown(SiafundCount)
}
