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
	TargetWindow          = 5000
)

var MaxAdjustmentUp = big.NewRat(10005, 10000)
var MaxAdjustmentDown = big.NewRat(9995, 10000)
var SurpassThreshold = big.NewRat(105, 100)

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
	r, s *big.Int
}

type Block struct {
	Version      uint16
	ParentBlock  BlockID
	Timestamp    Timestamp
	Nonce        uint32 // may or may not be needed
	MinerAddress CoinAddress
	MerkleRoot   Hash
	Transactions []Transaction
}

type Transaction struct {
	Version       uint16
	ArbitraryData []byte
	MinerFee      Currency
	Inputs        []Input
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
	CoveredFields  CoveredFields
	Signature      Signature
}

type CoveredFields struct {
	Version         bool
	ArbitraryData   bool
	MinerFee        bool
	Inputs, Outputs []uint8 // each element indicates an index which is signed.
	Contracts       []uint8
	FileProofs      []uint8
}

// Not enough flexibility in payments?  With the Start and End times, the only
// problem is if the client wishes for the rewards or penalties to scale as
// more are submitted or missed, and if they want things to scale harder in the
// case of consecutive misses.
type FileContract struct {
	ContractFund       Currency
	FileMerkleRoot     Hash
	FileSize           uint64 // probably in bytes, which means the last element in the merkle tree may not be exactly 64 bytes.
	Start, End         BlockHeight
	ChallengeFrequency uint32 // size of window, one window at a time
	Tolerance          uint32 // number of missed proofs before triggering unsuccessful termination
	ValidProofPayout   Currency
	ValidProofAddress  CoinAddress
	MissedProofPayout  Currency
	MissedProofAddress CoinAddress
	SuccessAddress     CoinAddress
	FailureAddress     CoinAddress
}

type StorageProof struct {
	ContractID ContractID
	Segment    [SegmentSize]byte
	HashSet    []*Hash
}

func (b *Block) ID() BlockID {
	return BlockID(HashBytes(MarshalAll(
		uint64(b.Version),
		Hash(b.ParentBlock),
		uint64(b.Timestamp),
		uint64(b.Nonce),
		Hash(b.MinerAddress),
		Hash(b.MerkleRoot),
	)))
}
