package sia

const (
	HashSize      = 32
	PublicKeySize = 32
	SignatureSize = 32
	SegmentSize   = 32
)

type (
	Hash      [HashSize]byte
	PublicKey [PublicKeySize]byte
	Signature [SignatureSize]byte

	Time     uint32
	Currency uint64

	BlockID    Hash
	OutputID   Hash // An output id points to a specific output.
	ContractID Hash
	TransactionID Hash
	Address    Hash // An address points to spend conditions.
)

type Block struct {
	Version      uint16
	Prevblock    BlockID
	Timestamp    Time
	Nonce        uint32 // may or may not be needed
	MinerAddress Address
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
	Signatures    []Signature
}

type Input struct {
	OutputID        OutputID // the source of coins for the input
	SpendConditions SpendConditions
}

type Output struct {
	Value     Currency // how many coins are in the output
	SpendHash Address
}

type SpendConditions struct {
	TimeLock      Time
	NumSignatures uint8
	PublicKeys    []PublicKey
}

type Signatures struct {
	InputID        OutputID // the OutputID of the Input that this signature is addressing. Using the index has also been considered.
	PublicKeyIndex uint8
	TimeLock       Time
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
	Start, End         Time
	ChallengeFrequency uint32 // size of window, one window at a time
	Tolerance          uint32 // number of missed proofs before triggering unsuccessful termination
	ValidProofPayout   Currency
	ValidProofAddress  Address
	MissedProofPayout  Currency
	MissedProofAddress Address
	SuccessAddress     Address
	FailureAddress     Address
}

type StorageProof struct {
	ContractID ContractID
	Segment    [SegmentSize]byte
	HashSet    []*Hash
}
