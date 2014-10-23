package undefined

import (
	"github.com/NebulousLabs/Sia/siacrypto"
)

type Version uint16
type Time uint32
type Currency uint64
type Address siacrypto.Hash

type Block struct {
	Version Version
	Prevblock siacrypto.Hash
	Timestamp Time
	Nonce uint32 // may or may not be needed
	MinerAddress Address
	MerkleRoot siacrypto.Hash
	Transactions []Transaction
}

type Transaction struct {
	Version Version
	ArbitraryData []byte
	MinerFee Currency
	Inputs []Input
	Outputs []Output
	FileContracts []FileContract
	StorageProofs []StorageProof
	Signatures []Signature
}

type Input struct {
	OutputID Address // the source of coins for the input
	SpendConditions SpendConditions
}

type Output struct {
	Value Currency // how many coins are in the output
	SpendHash siacrypto.Hash // is not an address
}

type SpendConditions struct {
	TimeLock Time
	RequiredSignatures uint8
	PublicKeys []siacrypto.PublicKey
}

type Signatures struct {
	InputID siacrypto.Hash // the OutputID of the Input that this signature is addressing. Using the index has also been considered.
	PublicKeyIndex uint8
	TimeLock Time
	CoveredFields CoveredFields
	Signature siacrypto.Signature
}

type CoveredFields struct {
	TransactionVersion bool
	ArbitraryData bool
	MinerFee bool
	Inputs []uint8 // each element indicates an input index which is signed.
	Outputs []uint8
	Contracts []uint8
	FileProofs []uint8
}

// Not enough flexibility in payments?  With the Start and End times, the only
// problem is if the client wishes for the rewards or penalties to scale as
// more are submitted or missed, and if they want things to scale harder in the
// case of consecutive misses.
type FileContract struct {
	ContractFund Currency
	FileMerkleRoot siacrypto.Hash
	FileSize uint64 // probably in bytes, which means the last element in the merkle tree may not be exactly 64 bytes.
	ContractStart Time
	ContractEnd Time
	ChallengeFrequency Time // might cause problems to use the same Time that's used everywhere else, might need to be a block number or have different rules if using unicode-style time.
	FailureTolerance uint32 // number of missed proofs before triggering unsuccessful termination
	ValidProofPayout Currency
	ValidProofPayoutAddress Address
	MissedProofPayout Currency
	MissedProofPayoutAddress Address
	SuccessfulTerminationAddress Address
	UnsuccessfulTerminationAddress Address
}

type StorageProof struct {
	ContractID hash
	StorageProofFileSegment []byte // the 64- bytes that form the leaf of the merkle tree that's been selected to be proven on.
	StorageProofHashSet []siacrypto.Hash
}
