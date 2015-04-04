package types

// transaction.go defines the transaction type and all of the sub-fields of the
// transaction, as well as providing helper functions for working with
// transactions. The various IDs are designed such that, in a legal blockchain,
// it is cryptographically unlikely that any two objects would share an id.

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

type (
	Siafund Currency // arbitrary-precision unsigned integer

	// A Specifier is a fixed-length string that serves two purposes. In the
	// wire protocol, they are used to identify a particular encoding
	// algorithm, signature algorithm, etc. This allows nodes to communicate on
	// their own terms; for example, to reduce bandwidth costs, a node might
	// only accept compressed messages.
	//
	// Internally, Specifiers are used to guarantee unique IDs. Various
	// consensus types have an associated ID, calculated by hashing the data
	// contained in the type. By prepending the data with Specifier, we can
	// guarantee that distinct types will never produce the same hash.
	Specifier [16]byte

	// IDs are used to refer to a type without revealing its contents. They
	// are constructed by hashing specific fields of the type, along with a
	// Specifier. While all of these types are hashes, defining type aliases
	// gives us type safety and makes the code more readable.
	SiacoinOutputID crypto.Hash
	SiafundOutputID crypto.Hash
	FileContractID  crypto.Hash

	// An UnlockHash is a specially constructed hash of the UnlockConditions
	// type. "Locked" values can be unlocked by providing the UnlockConditions
	// that hash to a given UnlockHash. See SpendConditions.UnlockHash for
	// details on how the UnlockHash is constructed.
	UnlockHash crypto.Hash

	// A Transaction is an atomic component of a block. Transactions can contain
	// inputs and outputs, file contracts, storage proofs, and even arbitrary
	// data. They can also contain signatures to prove that a given party has
	// approved the transaction, or at least a particular subset of it.
	//
	// Transactions can depend on other previous transactions in the same block,
	// but transactions cannot spend outputs that they create or otherwise be
	// self-dependent.
	Transaction struct {
		SiacoinInputs            []SiacoinInput
		SiacoinOutputs           []SiacoinOutput
		FileContracts            []FileContract
		FileContractTerminations []FileContractTermination
		StorageProofs            []StorageProof
		SiafundInputs            []SiafundInput
		SiafundOutputs           []SiafundOutput
		MinerFees                []Currency
		ArbitraryData            []string
		Signatures               []TransactionSignature
	}

	// A SiacoinInput consumes a SiacoinOutput and adds the siacoins to the set of
	// siacoins that can be spent in the transaction. The ParentID points to the
	// output that is getting consumed, and the UnlockConditions contain the rules
	// for spending the output. The UnlockConditions must match the UnlockHash of
	// the output.
	SiacoinInput struct {
		ParentID         SiacoinOutputID
		UnlockConditions UnlockConditions
	}

	// A SiacoinOutput holds a volume of siacoins. Outputs must be spent
	// atomically; that is, they must all be spent in the same transaction. The
	// UnlockHash is the hash of the UnlockConditions that must be fulfilled
	// in order to spend the output.
	SiacoinOutput struct {
		Value      Currency
		UnlockHash UnlockHash
	}

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

	// A SiafundInput consumes a SiafundOutput and adds the siafunds to the set of
	// siafunds that can be spent in the transaction. The ParentID points to the
	// output that is getting consumed, and the UnlockConditions contain the rules
	// for spending the output. The UnlockConditions must match the UnlockHash of
	// the output.
	SiafundInput struct {
		ParentID         SiafundOutputID
		UnlockConditions UnlockConditions
	}

	// A SiafundOutput holds a volume of siafunds. Outputs must be spent
	// atomically; that is, they must all be spent in the same transaction. The
	// UnlockHash is the hash of a set of UnlockConditions that must be fulfilled
	// in order to spend the output.
	//
	// When the SiafundOutput is spent, a SiacoinOutput is created, where:
	//
	//     SiacoinOutput.Value := (SiafundPool - ClaimStart) / 10,000
	//     SiacoinOutput.UnlockHash := SiafundOutput.ClaimUnlockHash
	//
	// When a SiafundOutput is put into a transaction, the ClaimStart must always
	// equal zero. While the transaction is being processed, the ClaimStart is set
	// to the value of the SiafundPool.
	SiafundOutput struct {
		Value           Currency
		UnlockHash      UnlockHash
		ClaimUnlockHash UnlockHash
		ClaimStart      Currency
	}
)

// These Specifiers are used internally when calculating a type's ID. See
// Specifier for more details.
var (
	SpecifierSiacoinOutput                 = Specifier{'s', 'i', 'a', 'c', 'o', 'i', 'n', ' ', 'o', 'u', 't', 'p', 'u', 't'}
	SpecifierFileContract                  = Specifier{'f', 'i', 'l', 'e', ' ', 'c', 'o', 'n', 't', 'r', 'a', 'c', 't'}
	SpecifierFileContractTerminationPayout = Specifier{'f', 'i', 'l', 'e', ' ', 'c', 'o', 'n', 't', 'r', 'a', 'c', 't', ' ', 't'}
	SpecifierStorageProofOutput            = Specifier{'s', 't', 'o', 'r', 'a', 'g', 'e', ' ', 'p', 'r', 'o', 'o', 'f'}
	SpecifierSiafundOutput                 = Specifier{'s', 'i', 'a', 'f', 'u', 'n', 'd', ' ', 'o', 'u', 't', 'p', 'u', 't'}
)

// ID returns the id of a transaction, which is taken by marshalling all of the
// fields except for the signatures and taking the hash of the result.
func (t Transaction) ID() crypto.Hash {
	tBytes := encoding.MarshalAll(
		t.SiacoinInputs,
		t.SiacoinOutputs,
		t.FileContracts,
		t.FileContractTerminations,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.MinerFees,
		t.ArbitraryData,
	)

	return crypto.HashBytes(tBytes)
}

// SiacoinOutputID returns the ID of a siacoin output at the given index,
// which is calculated by hashing the concatenation of the SiacoinOutput
// Specifier, all of the fields in the transaction (except the signatures),
// and output index.
func (t Transaction) SiacoinOutputID(i int) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		SpecifierSiacoinOutput,
		t.SiacoinInputs,
		t.SiacoinOutputs,
		t.FileContracts,
		t.FileContractTerminations,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.MinerFees,
		t.ArbitraryData,
		i,
	))
}

// FileContractID returns the ID of a file contract at the given index, which
// is calculated by hashing the concatenation of the FileContract Specifier,
// all of the fields in the transaction (except the signatures), and the
// contract index.
func (t Transaction) FileContractID(i int) FileContractID {
	return FileContractID(crypto.HashAll(
		SpecifierFileContract,
		t.SiacoinInputs,
		t.SiacoinOutputs,
		t.FileContracts,
		t.FileContractTerminations,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.MinerFees,
		t.ArbitraryData,
		i,
	))
}

// SiafundOutputID returns the ID of a SiafundOutput at the given index, which
// is calculated by hashing the concatenation of the SiafundOutput Specifier,
// all of the fields in the transaction (except the signatures), and output
// index.
func (t Transaction) SiafundOutputID(i int) SiafundOutputID {
	return SiafundOutputID(crypto.HashAll(
		SpecifierSiafundOutput,
		t.SiacoinInputs,
		t.SiacoinOutputs,
		t.FileContracts,
		t.FileContractTerminations,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.MinerFees,
		t.ArbitraryData,
		i,
	))
}

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

// SiaClaimOutputID returns the ID of the SiacoinOutput that is created when
// the siafund output is spent. The ID is the hash the SiafundOutputID.
func (id SiafundOutputID) SiaClaimOutputID() SiacoinOutputID {
	return SiacoinOutputID(crypto.HashObject(id))
}

// Tax returns the amount of Currency that will be taxed from fc.
func (fc FileContract) Tax() Currency {
	return fc.Payout.MulFloat(SiafundPortion).RoundDown(SiafundCount)
}
