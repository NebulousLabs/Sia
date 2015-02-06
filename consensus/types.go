package consensus

// TODO: Complete non-adversarial test coverage, partial adversarial test
// coverage.

import (
	"math/big"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
)

type (
	Timestamp   int64
	BlockHeight uint64
	Siafund     uint64

	Identifier [16]byte
	Signature  []byte

	BlockID        hash.Hash
	OutputID       hash.Hash
	FileContractID hash.Hash
	CoinAddress    hash.Hash
	Target         hash.Hash
)

// A Currency is a 128-bit unsigned integer. Currency operations are performed
// via math/big.
//
// The Currency object also keeps track of whether an overflow has occurred
// during arithmetic operations. Once the 'overflow' flag has been set to
// true, all subsequent operations will return an error, and the result of the
// operation is undefined. This flag can never be reset; a new Currency must
// be created. Callers can also manually check for overflow using the Overflow
// method.
type Currency struct {
	i  big.Int
	of bool // has an overflow ever occurred?
}

// Identifiers that get used when determining the IDs of various elements in
// the consensus state.
var (
	FileContractIdentifier  = Identifier{'f', 'i', 'l', 'e', ' ', 'c', 'o', 'n', 't', 'r', 'a', 'c', 't'}
	SiacoinOutputIdentifier = Identifier{'s', 'i', 'a', 'c', 'o', 'i', 'n', ' ', 'o', 'u', 't', 'p', 'u', 't'}
	SiafundOutputIdentifier = Identifier{'s', 'i', 'a', 'f', 'u', 'n', 'd', ' ', 'o', 'u', 't', 'p', 'u', 't'}
)

// Identifiers for the types of signatures that are recognized during signature
// validation.
var (
	SignatureEntropy = Identifier{'e', 'n', 't', 'r', 'o', 'p', 'y'}
	SignatureEd25519 = Identifier{'e', 'd', '2', '5', '5', '1', '9'}
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

// A Transaction is an update to the state of the network, can move money
// around, make contracts, etc.
type Transaction struct {
	SiacoinInputs  []SiacoinInput
	SiacoinOutputs []SiacoinOutput
	FileContracts  []FileContract
	StorageProofs  []StorageProof
	SiafundInputs  []SiafundInput
	SiafundOutputs []SiafundOutput
	MinerFees      []Currency
	ArbitraryData  []string
	Signatures     []TransactionSignature
}

// An Input contains the ID of the output it's trying to spend, and the spend
// conditions that unlock the output.
type SiacoinInput struct {
	OutputID        OutputID
	SpendConditions SpendConditions
}

// SpendConditions is a timelock and a set of public keys that are used to
// unlock ouptuts.
//
// The public keys come first so that when producing the CoinAddress, the
// TimeLock and the NumSignatures can be padded for privacy. If all of the
// public keys are known, it is more or less trivial to grind the TimeLock and
// the NumSignatures because each field has a low amount of entropy. You can
// protect the fields with privacy however, by using the scheme [Timelock]
// [Random Data] [Actual Public Keys...] [Random Data] [NumSignatures]. This
// allows one to reveal all or many of the public keys without being required
// to expose the TimeLock and NumSigantures.
type SpendConditions struct {
	TimeLock      BlockHeight
	PublicKeys    []SiaPublicKey
	NumSignatures uint64
}

// An Output contains a volume of currency and a 'CoinAddress', which is just a
// hash of the spend conditions which unlock the output.
type SiacoinOutput struct {
	Value     Currency
	SpendHash CoinAddress
}

// A FileContract contains the information necessary to enforce that a host
// stores a file.
type FileContract struct {
	FileMerkleRoot     hash.Hash
	FileSize           uint64
	Start, End         BlockHeight
	Payout             Currency
	ValidProofAddress  CoinAddress
	MissedProofAddress CoinAddress
}

// A StorageProof contains a segment and the HashSet required to prove that the
// segment is a part of the data in the FileMerkleRoot of the FileContract that
// the storage proof fulfills.
type StorageProof struct {
	FileContractID FileContractID
	Segment        [hash.SegmentSize]byte
	HashSet        []hash.Hash
}

// A SiafundInput is close to a SiacoinInput, except that the asset being spent
// is a SiaFund.
type SiafundInput struct {
	OutputID        OutputID
	SpendConditions SpendConditions
}

// A SiafundOutput contains a value and a spend hash like the SiacoinOutput,
// but it also contians a ClaimDestination and a claimStart. The
// ClaimDestination is the address that will receive siacoins when the siafund
// output is spent. The claimStart will be comapred to the SiafundPool to
// figure out how many siacoins the ClaimDestination will receive. The
// claimStart is always set internally, and therefore is not an exported field.
// Upon input, it must always equal zero.
type SiafundOutput struct {
	Value            Currency
	SpendHash        CoinAddress
	ClaimDestination CoinAddress
	ClaimStart       Currency // TODO: Update marshalling to ignore this unexported field.
}

// A SiaPublicKey is a public key prefixed by an identifier. The identifier
// details the algorithm used for sigining and verification, and the byte slice
// contains the actual public key. Doing things this way makes it easy to
// support multiple types of sigantures, and makes it easier to hardfork new
// signatures into the codebase.
type SiaPublicKey struct {
	Algorithm Identifier
	Key       []byte
}

// A TransactionSignature signs a single input to a transaction to help fulfill
// the unlock conditions of the transaction. It points to an input, a
// particular public key, has a timelock, and also indicates which parts of the
// transaction have been signed.
type TransactionSignature struct {
	InputID        OutputID // the OutputID of the Input that this signature is addressing.
	PublicKeyIndex uint64
	TimeLock       BlockHeight
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
	WholeTransaction bool
	SiacoinInputs    []uint64
	SiacoinOutputs   []uint64
	FileContracts    []uint64
	StorageProofs    []uint64
	SiafundInputs    []uint64
	SiafundOutputs   []uint64
	MinerFees        []uint64
	ArbitraryData    []uint64
	Signatures       []uint64
}

// CalculateCoinbase takes a height and from that derives the coinbase.
func CalculateCoinbase(height BlockHeight) (c Currency) {
	base := InitialCoinbase - uint64(height)
	if base < MinimumCoinbase {
		base = MinimumCoinbase
	}

	// Have to do error checking on NewCurrency, unfortunately.
	c, err := NewCurrency(new(big.Int).Mul(big.NewInt(int64(base)), CoinbaseAugment))
	if err != nil {
		if DEBUG {
			panic("err during CaluculateCoinbase???")
		}
	}
	return
}

// ID returns the id of a block, which is calculated by concatenating the
// parent block id, the block nonce, and the block merkle root and taking the
// hash.
func (b Block) ID() BlockID {
	return BlockID(hash.HashAll(
		b.ParentID,
		b.Nonce,
		b.MerkleRoot(),
	))
}

// MerkleRoot calculates the merkle root of the block. The leaves of the merkle
// tree are composed of the Timestamp, the set of miner outputs (one leaf), and
// all of the transactions (many leaves).
func (b Block) MerkleRoot() hash.Hash {
	leaves := []hash.Hash{
		hash.HashObject(b.Timestamp),
	}
	for _, payout := range b.MinerPayouts {
		leaves = append(leaves, hash.HashObject(payout))
	}
	for _, txn := range b.Transactions {
		leaves = append(leaves, hash.HashObject(txn))
	}
	return hash.MerkleRoot(leaves)
}

// MinerPayoutID returns the ID of the payout at the given index.
func (b Block) MinerPayoutID(i int) OutputID {
	return OutputID(hash.HashAll(
		b.ID(),
		i,
	))
}

// FileContractID returns the id of a file contract given the index of the
// contract. The id is derived by marshalling all of the fields in the
// transaction except for the signatures and then appending the string "file
// contract" and the index of the contract.
func (t Transaction) FileContractID(i int) FileContractID {
	return FileContractID(hash.HashAll(
		FileContractIdentifier,
		t.SiacoinInputs,
		t.SiacoinOutputs,
		t.FileContracts,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.MinerFees,
		t.ArbitraryData,
		i,
	))
}

// SiacoinOutputID gets the id of an output in the transaction, which is
// derived from marshalling all of the fields in the transaction except for the
// signatures and then appending the string "siacoin output" and the index of
// the output.
func (t Transaction) SiacoinOutputID(i int) OutputID {
	return OutputID(hash.HashAll(
		SiacoinOutputIdentifier,
		t.SiacoinInputs,
		t.SiacoinOutputs,
		t.FileContracts,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.MinerFees,
		t.ArbitraryData,
		i,
	))
}

// StorageProofOutputID returns the OutputID of the output created during the
// window index that was active at height 'height'.
func (fcid FileContractID) StorageProofOutputID(proofValid bool) (outputID OutputID) {
	outputID = OutputID(hash.HashAll(
		fcid,
		proofValid,
	))
	return
}

// SiafundOutputID returns the id of the siafund output that was specified and
// index `i` in the transaction.
func (t Transaction) SiafundOutputID(i int) OutputID {
	return OutputID(hash.HashAll(
		SiafundOutputIdentifier,
		t.SiacoinInputs,
		t.SiacoinOutputs,
		t.FileContracts,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.MinerFees,
		t.ArbitraryData,
		i,
	))
}

// SiaClaimOutputID returns the id of the siacoin output that is created when
// the siafund output gets spent.
func (id OutputID) SiaClaimOutputID() OutputID {
	return OutputID(hash.HashAll(
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
func (t Transaction) SigHash(i int) hash.Hash {
	cf := t.Signatures[i].CoveredFields
	var signedData []byte
	if cf.WholeTransaction {
		signedData = encoding.MarshalAll(
			t.SiacoinInputs,
			t.SiacoinOutputs,
			t.FileContracts,
			t.StorageProofs,
			t.SiafundInputs,
			t.SiafundOutputs,
			t.MinerFees,
			t.ArbitraryData,
			t.Signatures[i].InputID,
			t.Signatures[i].PublicKeyIndex,
			t.Signatures[i].TimeLock,
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

	return hash.HashBytes(signedData)
}

// CoinAddress calculates the root hash of a merkle tree of the SpendConditions
// object. The leaves of this tree are formed by taking the [TimeLock]
// [Pubkeys...] [NumSignatures].
func (sc SpendConditions) CoinAddress() CoinAddress {
	leaves := []hash.Hash{
		hash.HashObject(sc.TimeLock),
	}
	for i := range sc.PublicKeys {
		leaves = append(leaves, hash.HashObject(sc.PublicKeys[i]))
	}
	leaves = append(leaves, hash.HashObject(sc.NumSignatures))
	return CoinAddress(hash.MerkleRoot(leaves))
}
