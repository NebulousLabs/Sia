package sia

import (
	"crypto/ecdsa"
	"math/big"
)

const (
	HashSize      = 32
	PublicKeySize = 32
	SignatureSize = 32
	SegmentSize   = 32

	TargetSecondsPerBlock = 300
	TargetWindow          = BlockHeight(5000) // Number of blocks to use when calculating difficulty.

	FutureThreshold = Timestamp(2 * 60 * 60) // Seconds into the future block timestamps are valid.
)

var MaxAdjustmentUp = big.NewRat(10005, 10000)
var MaxAdjustmentDown = big.NewRat(9995, 10000)
var SurpassThreshold = big.NewRat(5, 100)

type (
	Hash      [HashSize]byte
	PublicKey ecdsa.PublicKey

	Timestamp   int64
	BlockHeight uint32
	BlockWeight *big.Rat // inverse of target
	Currency    uint64

	BlockID       Hash
	OutputID      Hash // An output id points to a specific output.
	ContractID    Hash
	TransactionID Hash
	CoinAddress   Hash // An address points to spend conditions.
	Target        Hash
)

type Signature struct {
	R, S *big.Int
}

// Eventually, a block will be separate from a block header, and a block header
// will contian nothing more than a parent hash, a 64 bit nonce, and a child
// hash. The child hash will be a merkle tree of different blocks that share a
// header, for merge mining. The blocks themselves will contain timestamps and
// additonal nonces as needed.
type Block struct {
	ParentBlock  BlockID
	Timestamp    Timestamp
	Nonce        uint64
	MinerAddress CoinAddress
	MerkleRoot   Hash
	Transactions []Transaction
}

type Transaction struct {
	ArbitraryData []byte
	Inputs        []Input
	MinerFees     []Currency
	Outputs       []Output
	FileContracts []FileContract
	StorageProofs []StorageProof
	Signatures    []TransactionSignature
}

type Input struct {
	OutputID        OutputID // the source of coins for the input
	SpendConditions SpendConditions
}

type Output struct {
	Value     Currency // how many coins are in the output
	SpendHash CoinAddress
}

type SpendConditions struct {
	TimeLock      BlockHeight
	NumSignatures uint8
	PublicKeys    []PublicKey
}

type TransactionSignature struct {
	InputID        OutputID // the OutputID of the Input that this signature is addressing. Using the index has also been considered.
	PublicKeyIndex uint8
	TimeLock       BlockHeight
	// CoveredFields  CoveredFields
	Signature Signature
}

/*
type CoveredFields struct {
	ArbitraryData bool
	MinerFees     []uint8 // each element indicates an index which is signed.
	Inputs        []uint8
	Outputs       []uint8
	Contracts     []uint8
	FileProofs    []uint8
}
*/

// Not enough flexibility in payments?  With the Start and End times, the only
// problem is if the client wishes for the rewards or penalties to scale as
// more are submitted or missed, and if they want things to scale harder in the
// case of consecutive misses.
type FileContract struct {
	ContractFund       Currency
	FileMerkleRoot     Hash
	FileSize           uint64 // probably in bytes, which means the last element in the merkle tree may not be exactly 64 bytes.
	Start, End         BlockHeight
	ChallengeFrequency BlockHeight // size of window, one window at a time
	Tolerance          uint32      // number of missed proofs before triggering unsuccessful termination
	ValidProofPayout   Currency
	ValidProofAddress  CoinAddress
	MissedProofPayout  Currency
	MissedProofAddress CoinAddress
}

type StorageProof struct {
	ContractID ContractID
	Segment    [SegmentSize]byte
	HashSet    []Hash
}

// ID returns the Block's unique identifier, generated from the hash of its internal data.
// Transactions are not included in the hash.
func (b *Block) ID() BlockID {
	return BlockID(HashBytes(MarshalAll(
		b.ParentBlock,
		b.Timestamp,
		b.Nonce,
		b.MinerAddress,
		b.MerkleRoot,
	)))
}

func (b *Block) subsidyID() OutputID {
	bid := b.ID()
	return OutputID(HashBytes(append(bid[:], []byte("blockreward")...)))
}

// SigHash returns the hash of a transaction for a specific index.
// The index determines which TransactionSignature is included in the hash.
func (t *Transaction) SigHash(i int) Hash {
	return HashBytes(MarshalAll(
		t.ArbitraryData,
		t.Inputs,
		t.MinerFees,
		t.Outputs,
		t.FileContracts,
		t.StorageProofs,
		t.Signatures[i].InputID,
		t.Signatures[i].PublicKeyIndex,
		t.Signatures[i].TimeLock,
	))
}

// transaction.ouptutID() takes the index of the output and returns the
// output's ID.
func (t *Transaction) outputID(index int) OutputID {
	return OutputID(HashBytes(append(Marshal(t), append([]byte("coinsend"), Marshal(uint64(index))...)...))) // typecast to uint64 shouldn't be needed.
}

// SpendConditions.CoinAddress() calculates the root hash of a merkle tree of the
// SpendConditions object, using the timelock, number of signatures required,
// and each public key as leaves.
func (sc *SpendConditions) CoinAddress() CoinAddress {
	tlHash := HashBytes(Marshal(sc.TimeLock))
	nsHash := HashBytes(Marshal(sc.NumSignatures))
	pkHashes := make([]Hash, len(sc.PublicKeys))
	for i := range sc.PublicKeys {
		pkHashes[i] = HashBytes(Marshal(sc.PublicKeys[i]))
	}
	leaves := append([]Hash{tlHash, nsHash}, pkHashes...)
	return CoinAddress(MerkleRoot(leaves))
}

// Returns the id of a file contract given the transaction it appears in and
// the index of the contract within the transaction.
func (t *Transaction) fileContractID(index int) ContractID {
	return ContractID(HashBytes(append(Marshal(t), append([]byte("contract"), Marshal(uint64(index))...)...))) // typecast to uint64 shouldn't be needed.
}

// Returns the index of the current window of a contract, given the current
// height of the ConsensusState.
func (fc *FileContract) windowIndex(currentHeight BlockHeight) BlockHeight {
	return (currentHeight - fc.Start) / fc.ChallengeFrequency
}

// FileContract.storageProofOutput() returns the OutputID of the output created during window index
func (t *Transaction) storageProofOutput(fileContractIndex int, height BlockHeight, proofValid bool) OutputID {
	fileContractID := t.fileContractID(fileContractIndex)
	proofString := proofString(proofValid)
	windowIndex := t.FileContracts[fileContractIndex].windowIndex(height)
	return OutputID(HashBytes(append(fileContractID[:], append(proofString, Marshal(windowIndex)...)...)))
}

// MarshalSia implements the Marshaler interface for Signatures.
func (s *Signature) MarshalSia() []byte {
	if s.R == nil || s.S == nil {
		return []byte{0, 0}
	}
	// pretend Signature is a tuple of []bytes
	// this lets us use Marshal instead of doing manual length-prefixing
	return Marshal(struct{ R, S []byte }{s.R.Bytes(), s.S.Bytes()})
}

// MarshalSia implements the Unmarshaler interface for Signatures.
func (s *Signature) UnmarshalSia(b []byte) int {
	// inverse of the struct trick used in Signature.MarshalSia
	str := struct{ R, S []byte }{}
	if Unmarshal(b, &str) != nil {
		return 0
	}
	s.R = new(big.Int).SetBytes(str.R)
	s.S = new(big.Int).SetBytes(str.S)
	return len(str.R) + len(str.S) + 2
}

// MarshalSia implements the Marshaler interface for PublicKeys.
func (pk PublicKey) MarshalSia() []byte {
	if pk.X == nil || pk.Y == nil {
		return []byte{0, 0}
	}
	// see Signature.MarshalSia
	return Marshal(struct{ X, Y []byte }{pk.X.Bytes(), pk.Y.Bytes()})
}

// MarshalSia implements the Unmarshaler interface for PublicKeys.
func (pk *PublicKey) UnmarshalSia(b []byte) int {
	// see Signature.UnmarshalSia
	str := struct{ X, Y []byte }{}
	if Unmarshal(b, &str) != nil {
		return 0
	}
	pk.X = new(big.Int).SetBytes(str.X)
	pk.Y = new(big.Int).SetBytes(str.Y)
	return len(str.X) + len(str.Y) + 2
}

// proofString() returns the string to be used when generating the output id of
// a valid proof if bool is set to true, and it returns the string to be used
// in a missed proof if the bool is set to false.
func proofString(proofValid bool) []byte {
	if proofValid {
		return []byte("validproof")
	} else {
		return []byte("missedproof")
	}
}

// terminationString() returns the string to be used when generating the output
// id of a successful terminated contract if the bool is set to true, and of an
// unsuccessful termination if the bool is set to false.
func terminationString(success bool) []byte {
	if success {
		return []byte("successfultermination")
	} else {
		return []byte("unsuccessfultermination")
	}
}
