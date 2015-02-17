package consensus

import (
	"bytes"
	"math/big"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

type (
	Timestamp   uint64
	BlockHeight uint64
	Siafund     Currency // arbitrary-precision unsigned integer

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

	// The Signature type is arbitrary-length to enable a variety of signature
	// algorithms.
	Signature string

	// IDs are used to refer to a type without revealing its contents. They
	// are constructed by hashing specific fields of the type, along with a
	// Specifier. While all of these types are hashes, defining type aliases
	// gives us type safety and makes the code more readable.
	BlockID         crypto.Hash
	SiacoinOutputID crypto.Hash
	SiafundOutputID crypto.Hash
	FileContractID  crypto.Hash

	// An UnlockHash is a specially constructed hash of the UnlockConditions
	// type. "Locked" values can be unlocked by providing the UnlockConditions
	// that hash to a given UnlockHash. See SpendConditions.UnlockHash for
	// details on how the UnlockHash is constructed.
	UnlockHash crypto.Hash

	// A Target is a hash that a block's ID must be "less than" in order for
	// the block to be considered valid. Miners vary the block's 'Nonce' field
	// in order to brute-force such an ID. The inverse of a Target is called
	// the "difficulty," because it is proportional to the amount of time
	// required to brute-force the Target.
	Target crypto.Hash
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

// These Specifiers enumerate the types of signatures that are recognized by
// this implementation. If a signature's type is unrecognized, the signature
// is treated as valid. Signatures using the special "entropy" type are always
// treated as invalid; see Consensus.md for more details.
var (
	SignatureEntropy = Specifier{'e', 'n', 't', 'r', 'o', 'p', 'y'}
	SignatureEd25519 = Specifier{'e', 'd', '2', '5', '5', '1', '9'}
)

// A Block is a summary of changes to the state that have occurred since the
// previous block. Blocks reference the ID of the previous block (their
// "parent"), creating the linked-list commonly known as the blockchain. Their
// primary function is to bundle together transactions on the network. Blocks
// are created by "miners," who collect transactions from other nodes, and
// then try to pick a Nonce that results in a block whose BlockID is below a
// given Target.
type Block struct {
	ParentID     BlockID
	Nonce        uint64
	Timestamp    Timestamp
	MinerPayouts []SiacoinOutput
	Transactions []Transaction
}

// A Transaction is an atomic component of a block. Transactions can contain
// inputs and outputs, file contracts, storage proofs, and even arbitrary
// data. They can also contain signatures to prove that a given party has
// approved the transaction, or at least a particular subset of it.
//
// Transactions can depend on other previous transactions in the same block,
// but transactions cannot spend outputs that they create or otherwise be
// self-dependent.
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
// the output.
type SiacoinInput struct {
	ParentID         SiacoinOutputID
	UnlockConditions UnlockConditions
}

// A SiacoinOutput holds a volume of siacoins. Outputs must be spent
// atomically; that is, they must all be spent in the same transaction. The
// UnlockHash is the hash of the UnlockConditions that must be fulfilled
// in order to spend the output.
type SiacoinOutput struct {
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
type FileContract struct {
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
type FileContractTermination struct {
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
type SiafundOutput struct {
	Value           Currency
	UnlockHash      UnlockHash
	ClaimUnlockHash UnlockHash
	ClaimStart      Currency
}

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
//
// TODO: use an enum here?
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

// CurrentTimestamp returns the current time as a Timestamp.
func CurrentTimestamp() Timestamp {
	return Timestamp(time.Now().Unix())
}

// CalculateCoinbase calculates the coinbase for a given height. The coinbase
// equation is:
//
//     coinbase := max(InitialCoinbase - height, MinimumCoinbase) * CoinbaseAugment
func CalculateCoinbase(height BlockHeight) (c Currency) {
	base := InitialCoinbase - uint64(height)
	if base < MinimumCoinbase {
		base = MinimumCoinbase
	}

	return NewCurrency64(base).Mul(NewCurrency(CoinbaseAugment))
}

// Int converts a Target to a big.Int.
func (t Target) Int() *big.Int {
	return new(big.Int).SetBytes(t[:])
}

// Rat converts a Target to a big.Rat.
func (t Target) Rat() *big.Rat {
	return new(big.Rat).SetInt(t.Int())
}

// Inverse returns the inverse of a Target as a big.Rat
func (t Target) Inverse() *big.Rat {
	return new(big.Rat).Inv(t.Rat())
}

// IntToTarget converts a big.Int to a Target.
func IntToTarget(i *big.Int) (t Target) {
	// i may overflow the maximum target.
	// In the event of overflow, return the maximum.
	if i.BitLen() > 256 {
		return RootDepth
	}
	b := i.Bytes()
	// need to preserve big-endianness
	offset := len(t[:]) - len(b)
	copy(t[offset:], b)
	return
}

// RatToTarget converts a big.Rat to a Target.
func RatToTarget(r *big.Rat) Target {
	// conversion to big.Int truncates decimal
	i := new(big.Int).Div(r.Num(), r.Denom())
	return IntToTarget(i)
}

// Tax returns the amount of Currency that will be taxed from fc.
func (fc FileContract) Tax() Currency {
	return fc.Payout.MulFloat(SiafundPortion).RoundDown(SiafundCount)
}

// UnlockHash calculates the root hash of a Merkle tree of the
// UnlockConditions object. The leaves of this tree are formed by taking the
// hash of the timelock, the hash of the public keys (one leaf each), and the
// hash of the number of signatures. The keys are put in the middle because
// Timelock and NumSignatures are both low entropy fields; they can be
// protected by having random public keys next to them.
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

// ID returns the ID of a Block, which is calculated by hashing the
// concatenation of the block's parent ID, nonce, and Merkle root.
func (b Block) ID() BlockID {
	return BlockID(crypto.HashAll(
		b.ParentID,
		b.Nonce,
		b.MerkleRoot(),
	))
}

// CheckTarget returns true if the block's ID meets the given target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// MerkleRoot calculates the Merkle root of a Block. The leaves of the Merkle
// tree are composed of the Timestamp, the miner outputs (one leaf per
// payout), and the transactions (one leaf per transaction).
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
// is calculated by hashing the concatenation of the BlockID and the payout
// index.
func (b Block) MinerPayoutID(i int) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		b.ID(),
		i,
	))
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

// SiaClaimOutputID returns the ID of the SiacoinOutput that is created when
// the siafund output is spent. The ID is the hash the SiafundOutputID.
func (id SiafundOutputID) SiaClaimOutputID() SiacoinOutputID {
	return SiacoinOutputID(crypto.HashObject(id))
}

// SigHash returns the hash of the fields in a transaction covered by a given
// signature. See CoveredFields for more details.
//
// TODO: make this take a CoveredFields instead?
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
