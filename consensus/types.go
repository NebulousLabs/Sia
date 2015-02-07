package consensus

// TODO: Luke should review this file and provide inputs for the variable names
// in all of the data structures. I tried thinning things out and clearing
// things up as much as possible.

import (
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

type (
	Timestamp   uint64
	BlockHeight uint64
	Siafund     Currency

	Specifier [16]byte
	Signature string

	// Each of these types all come back to a crypto.Hash, but provide
	// important type safety when passing the various hashes around. Having
	// everything be a different type means its less likely that a siacoin
	// output id will accidentally be used where a siafund output id is
	// intended.
	BlockID         crypto.Hash
	SiacoinOutputID crypto.Hash
	SiafundOutputID crypto.Hash
	FileContractID  crypto.Hash
	UnlockHash      crypto.Hash
	Target          crypto.Hash
)

// Currency is not defined here because it has special behavior. Currency is a
// 128 bit unsigned integer that will not overflow, and is encoded as 16 bytes.

// Specifiers that get used when determining the IDs of various elements in
// the consensus state.
var (
	SpecifierSiacoinOutput                 = Specifier{'s', 'i', 'a', 'c', 'o', 'i', 'n', ' ', 'o', 'u', 't', 'p', 'u', 't'}
	SpecifierFileContract                  = Specifier{'f', 'i', 'l', 'e', ' ', 'c', 'o', 'n', 't', 'r', 'a', 'c', 't'}
	SpecifierFileContractTerminationPayout = Specifier{'f', 'i', 'l', 'e', ' ', 'c', 'o', 'n', 't', 'r', 'a', 'c', 't', ' ', 't'}
	SpecifierStorageProofOutput            = Specifier{'s', 't', 'o', 'r', 'a', 'g', 'e', ' ', 'p', 'r', 'o', 'o', 'f'}
	SpecifierSiafundOutput                 = Specifier{'s', 'i', 'a', 'f', 'u', 'n', 'd', ' ', 'o', 'u', 't', 'p', 'u', 't'}
)

// Specifiers for the types of signatures that are recognized during signature
// validation.
var (
	SignatureEntropy = Specifier{'e', 'n', 't', 'r', 'o', 'p', 'y'}
	SignatureEd25519 = Specifier{'e', 'd', '2', '5', '5', '1', '9'}
)

// A Block contains all of the changes to the state that have occurred since
// the previous block. There are constraints that make it difficult and
// rewarding to find a block.
type Block struct {
	ParentID     BlockID
	Nonce        uint64
	Timestamp    Timestamp
	MinerPayouts []SiacoinOutput
	Transactions []Transaction
}

// A transaction is an atomic component of a block, a single update to the
// consensus set. Transactions can depend on other transactions earlier in the
// block, but transactions cannot spend outputs that they create or otherwise
// be self-dependent.
type Transaction struct {
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
// the output. The siacoins are added to the transactions total number of input
// coins, and can be spent in SiacoinOutputs, MinerFees, or FileContracts.
type SiacoinInput struct {
	ParentID         SiacoinOutputID
	UnlockConditions UnlockConditions
}

// A SiacoinOutput holds a volume of siacoins that must be spent atomically.
// The volume is specified in the field 'Value'. The UnlockHash is the hash of
// a set of UnlockConditions that must be fulfilled in order to spend the siacoin
// output.
type SiacoinOutput struct {
	Value      Currency
	UnlockHash UnlockHash
}

// A FileContract holds some party accountable for storing a file. The size of
// the file and the file merkle hash are stored in the contract, which can be
// used to verify a storage proof that the party must submit. The party must
// submit the storage proof in a block that is between BlockHeight 'Start' and
// 'End'. If the party submits a valid storage proof in a block between heights
// 'Start' and 'End', then an output is created that has the UnlockHash
// 'ValidProofAddress'. If the party does not submit a storage proof between
// these blocks, an output is created that has the UnlockHash
// 'MissedProofAddress'.
//
// Under normal circumstances, the Payout will be composed partially of coins
// by the uploader, and partially of coins by the party accountable for storing
// the file, which gives the party incentive not to lose the file. The
// 'ValidProofUnlockHash' will typically be spendable by party storing the
// file. The 'MissedProofUnlockHash' will either by spendable by the uploader,
// or will be spendable by nobody (the ZeroAddress).
//
// The volume of coins that get put in the output is not quite the whole
// Payout; 3.9% of the coins (rounded down to the nearest 10,000) get put into
// the SiafundPool, which is a set of siacoins that are only spendable by
// Siafund owners. The majority of Siafunds are owned by NebulousLabs, who
// employs the majority of developers working on Sia.
//
// The FileContract can be terminated early by submitting a
// FileContractTermination in a block. The FileContractTermination can be
// submitted any time that the FileContract is still a part of the consensus
// set. The TerminationHash is the hash of an UnlockConditions. Typical use
// will require signatures from both the uploader and the party storing the
// file in the contract, and will generally only be used if the file has been
// edited and needs a new contract to reflect the changes to the file.
type FileContract struct {
	FileSize              uint64
	FileMerkleRoot        crypto.Hash
	Start                 BlockHeight
	End                   BlockHeight
	Payout                Currency
	ValidProofUnlockHash  UnlockHash
	MissedProofUnlockHash UnlockHash
	TerminationHash       UnlockHash
}

// A FileContractTermination terminates a FileContract, removing it from the
// consensus set. The ParentID points to the FileContract being terminated, and
// the TerminationConditions are the conditions which enable the FileContract
// to be terminated. The hash of the TerminationConditions must match the
// TerminationHash in the FileContract. The PayoutReturn is a set of
// SiacoinOutputs which must sum to the Payout of the FileContract being
// terminated. The Payouts can have any Value and UnlockHash, and do not
// need to match the ValidProofUnlockHash or MissedProofUnlockHash of the
// original FileContract. The Payouts do need to sum to the original
// FileContract Payout.
//
// The typical use case is to edit files and resubmit the contracts.
type FileContractTermination struct {
	ParentID              FileContractID
	TerminationConditions UnlockConditions
	Payouts               []SiacoinOutput
}

// A StorageProof fulfills a FileContract, resulting in the Payout getting put
// in a SiacoinOutput with an UnlockHash of 'ValidProofUnlockHash' from the
// FileContract getting fulfilled.. The StorageProof must be submitted between
// the 'Start' and 'End' of the FileContract.
//
// The proof contains a specific segment of the file, which is chosen by using
// the id of the trigger block. The trigger block is the block at height
// 'Start' - 1. This provides a strong random number (can be manipulated, but
// manipulating is expensive) that everyone can see. Using a random number
// prevents the party storing the file from pregenerating the storage proof.
//
// The segment is provided in field 'Segment', and the field 'HashSet' contains
// the set of hashes required to prove that the segment is part of the file
// with the Merkle root 'FileMerkleRoot'.
//
// A transaction with a StorageProof cannot have any SiacoinOutputs,
// SiafundOutputs, or FileContracts. This is because StorageProofs can be
// invalidated by simple and accidental reorgs, which will subsequently
// invalidate the rest of the transaction, leaving the fields in limbo until
// there is certainty that there will not be another reorg.
type StorageProof struct {
	ParentID FileContractID
	Segment  [crypto.SegmentSize]byte
	HashSet  []crypto.Hash
}

// A SiafundInput consumes a SiafundOutput and adds the siafunds to the set of
// siafunds that can be spent in the transaction. The ParentID points to the
// output that is getting consumed, and the UnlockConditions contain the rules
// for spending the output. The UnlockConditions must match the UnlockHash of
// the output.
type SiafundInput struct {
	ParentID         SiafundOutputID
	UnlockConditions UnlockConditions
}

// A SiafundOutput holds a volume of siafunds that must be spent atomically.
// The volume is specified in the field 'Value'. The UnlockHash is the hash of
// the UnlockConditions that must be fulfilled in order to spend the siafund
// output. The siafunds are add to the transaction's total number of siafunds,
// which can be spent in SiafundOutputs.
//
// When the SiafundOutput is spent, a SiacoinOutput is created. The 'Value'
// field of the SiacoinOutput is determined by ((SiafundPool - ClaimStart) /
// 10,000). The 'UnlockHash' of the SiacoinOutput is set equal to the
// 'ClaimUnlockHash' in the SiafundOutput.
//
// When a SiafundOutput is put into a transaction, the ClaimStart must always
// equal zero. While the transaction is being processed, the ClaimStart is set
// equal to the value of the SiafundPool.
type SiafundOutput struct {
	Value           Currency
	UnlockHash      UnlockHash
	ClaimUnlockHash UnlockHash
	ClaimStart      Currency
}

// UnlockConditions is a set of conditions which must be met to execute an
// actions (such as spend a SiacoinOutput or terminate a FileContract). The
// block containing the UnlockConditions must have a height equal to or greater
// than 'Timelock'. 'NumSignatures' refers to the number of sigantures that
// must appear in the transaction to fulfill the UnlockConditions, and each
// signature must correspond to exactly one SiaPublicKey in the set of
// 'PublicKeys'.
//
// If NumSignatures is 0, the UnlockConditions are effectively "anyone can
// unlock." If NumSignatures > len(PublicKeys), then the UnlockConditions
// cannot be fulfilled under any circumstances. The SiaPublicKeys that compose
// PublicKeys do not need to use the same cryptographic algorithm, which means
// multisig UnlockConditions can be set up which use multiple different
// cryptographic algorithms, which is useful in case any of the algorithms is
// ever compromised.
//
// The UnlockHash is formed by making a Merkle Tree of the elements of the
// UnlockConditions, where the Timelock is one leaf, each public key is one
// leaf, and NumSignatures is one leaf. This allows individual elements of the
// UnlockConditions to be revealed without revealing all of the elements. The
// PublicKeys are put in the middle because Timelock and NumSignatures are each
// low entropy fields. They can be protected by having random public keys next
// to them. (The use case for revealing public keys but not timestamps or
// numsignatures is unknown, but it is available nonetheless).
type UnlockConditions struct {
	Timelock      BlockHeight
	PublicKeys    []SiaPublicKey
	NumSignatures uint64
}

// A SiaPublicKey is a public key prefixed by a specifier. The specifier
// indicates the algorithm used for sigining and verification, and the byte
// slice contains the actual public key. Unrecognized algorithms will always
// verify, which allows new algorithms to be soft forked into the protocol.
// This is useful for safeguarding against the cryptographic break of an
// algorithm in use, and useful for taking advantage of any new algorithms that
// get discovered.
type SiaPublicKey struct {
	Algorithm Specifier
	Key       []byte
}

// A TransactionSignature signs a single PublicKey in one of the
// UnlockConditions of the transaction. Which UnlockConditions is indicated by
// the ParentID, which is the same as the string representation of the ParentID
// of the element that presented the UnlockConditions. The PublicKeyIndex
// indicates which public key of the UnlockConditions is doing the signing. The
// Timelock prevents the TransactionSignature from being used until a certain
// block height. CoveredFields indicates which parts of the transaction have
// been signed, and the Signature contains the actual signature.
type TransactionSignature struct {
	InputID        string
	PublicKeyIndex uint64
	Timelock       BlockHeight
	CoveredFields  CoveredFields
	Signature      Signature
}

// The CoveredFields portion of a signature indicates which fields in the
// transaction have been covered by the signature. Each slice of elements in a
// transaction is represented by a slice of indices. The indicies must be
// sorted, must not repeat, and must point to elements that exist within the
// transaction. If 'WholeTransaction' is set to true, all other fields must be
// empty except for the Signatures field.
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

// CalculateCoinbase takes a height and from that derives the coinbase.
func CalculateCoinbase(height BlockHeight) (c Currency) {
	base := InitialCoinbase - uint64(height)
	if base < MinimumCoinbase {
		base = MinimumCoinbase
	}

	c, err := NewCurrency(new(big.Int).Mul(big.NewInt(int64(base)), CoinbaseAugment))
	if err != nil {
		if DEBUG {
			panic("err during CaluculateCoinbase?")
		}
	}
	return
}

// UnlockHash calculates the root hash of a Merkle tree of the UnlockConditions
// object. The leaves of this tree are formed by taking the hash of the
// timelock, the hash of the public keys (one leaf each), and the hash of the
// number of signatures.
func (uc UnlockConditions) UnlockHash() UnlockHash {
	leaves := []crypto.Hash{
		crypto.HashObject(uc.Timelock),
	}
	for i := range uc.PublicKeys {
		leaves = append(leaves, crypto.HashObject(uc.PublicKeys[i]))
	}
	leaves = append(leaves, crypto.HashObject(uc.NumSignatures))
	return UnlockHash(crypto.MerkleRoot(leaves))
}

// ID returns the id of a block, which is calculated by concatenating the
// parent block id, the block nonce, and the block merkle root and taking the
// hash.
func (b Block) ID() BlockID {
	return BlockID(crypto.HashAll(
		b.ParentID,
		b.Nonce,
		b.MerkleRoot(),
	))
}

// MerkleRoot calculates the merkle root of the block. The leaves of the merkle
// tree are composed of the Timestamp, the set of miner outputs (one leaf per
// payout), and all of the transactions (one leaf per transaction).
func (b Block) MerkleRoot() crypto.Hash {
	leaves := []crypto.Hash{
		crypto.HashObject(b.Timestamp),
	}
	for _, payout := range b.MinerPayouts {
		leaves = append(leaves, crypto.HashObject(payout))
	}
	for _, txn := range b.Transactions {
		leaves = append(leaves, crypto.HashObject(txn))
	}
	return crypto.MerkleRoot(leaves)
}

// MinerPayoutID returns the ID of the miner payout at the given index, which
// is derived by appending the index of the miner payout to the id of the
// block.
func (b Block) MinerPayoutID(i int) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		b.ID(),
		i,
	))
}

// SiacoinOutputID returns the id of a siacoin output given the index of the
// output. The id is derived by taking SpecifierSiacoinOutput, appending all of
// the fields in the transaction except the signatures, and then appending the
// index of the SiacoinOutput in the transaction and taking the hash.
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

// FileContractID returns the id of a file contract given the index of the
// contract. The id is derived by taking SpecifierFileContract, appending all
// of the fields in the transaction except the signatures, and then appending
// the index of the FileContract in the transaction and taking the hash.
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

// FileContractTerminationPayoutID returns the id of a file contract
// termination payout given the index of the payout in the termination. The id
// is derived by taking SpecifierFileContractTerminationPayout, appending the
// id of the file contract that is being terminated, and then appending the
// index of the payout in the termination and taking the hash.
func (fcid FileContractID) FileContractTerminationPayoutID(i int) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		SpecifierFileContractTerminationPayout,
		fcid,
		i,
	))
}

// StorageProofOutputID returns the id of the output created by a file contract
// given the status of the storage proof. The id is derived by taking
// SpecifierStorageProofOutput, appending the id of the file contract that the
// proof is for, and then appending a bool indicating whether the proof was
// valid or missed (true is valid, false is missed), then taking the hash.
func (fcid FileContractID) StorageProofOutputID(proofValid bool) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		SpecifierStorageProofOutput,
		fcid,
		proofValid,
	))
}

// SiafundOutputID returns the id of a SiafundOutput given the index of the
// output. The id is derived by taking SpecifierSiafundOutput, appending all of
// the fields in the transaction except the signatures, and then appending the
// index of the SiafundOutput in the transaction and taking the hash.
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

// SiaClaimOutputID returns the id of the SiacoinOutput that is created when
// the siafund output gets spent. The id is calculated by taking the hash of
// the id of the SiafundOutput.
func (id SiafundOutputID) SiaClaimOutputID() SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		id,
	))
}

// SigHash returns the hash of a transaction for a specific signature. `i` is
// the index of the signature for which the hash is being returned. If
// `WholeTransaction` is set to true for the siganture, then all of the
// transaction fields except the signatures are included in the transactions.
// If `WholeTransaction` is set to false, then the fees, inputs, ect. are all
// added individually. The signatures are added individually regardless of the
// value of `WholeTransaction`.
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
			t.Signatures[i].InputID,
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
