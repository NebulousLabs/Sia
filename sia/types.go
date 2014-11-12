package sia

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
)

const (
	HashSize      = 32
	PublicKeySize = 32
	SignatureSize = 32
	SegmentSize   = 64 // Size of smallest piece of a file which gets hashed when building the Merkle tree.

	BlockFrequency = 600               // In seconds.
	TargetWindow   = BlockHeight(2016) // Number of blocks to use when calculating the target.

	FutureThreshold = Timestamp(3 * 60 * 60) // Seconds into the future block timestamps are valid.
)

var MaxAdjustmentUp = big.NewRat(1001, 1000)
var MaxAdjustmentDown = big.NewRat(999, 1000)

type (
	Hash      [HashSize]byte
	PublicKey ecdsa.PublicKey

	Timestamp   int64
	BlockHeight uint64
	Currency    uint64

	BlockID       Hash
	OutputID      Hash // An output id points to a specific output.
	ContractID    Hash
	TransactionID Hash
	CoinAddress   Hash // An address is the hash of the spend conditions that unlock the output.
	Target        Hash
)

// A Signature follows the crypto/ecdsa golang standard for signatures.
// Eventually we plan to switch to a more standard library such as NaCl or
// OpenSSL.
type Signature struct {
	R, S *big.Int
}

// Eventually, the Block and the block header will be two separate structs.
// This will be put into practice when we implement merged mining.
type Block struct {
	ParentBlock  BlockID
	Timestamp    Timestamp
	Nonce        uint64
	MinerAddress CoinAddress
	MerkleRoot   Hash
	Transactions []Transaction
}

// A Transaction is an update to the state of the network, can move money
// around, make contracts, etc.
type Transaction struct {
	ArbitraryData []byte
	Inputs        []Input
	MinerFees     []Currency
	Outputs       []Output
	FileContracts []FileContract
	StorageProofs []StorageProof
	Signatures    []TransactionSignature
}

// An Input contains the ID of the output it's trying to spend, and the spend
// conditions that unlock the output.
type Input struct {
	OutputID        OutputID // the source of coins for the input
	SpendConditions SpendConditions
}

// An Output contains a volume of currency and a 'CoinAddress', which is just a
// hash of the spend conditions which unlock the output.
type Output struct {
	Value     Currency // how many coins are in the output
	SpendHash CoinAddress
}

// SpendConditions is a timelock and a set of public keys that are used to
// unlock ouptuts.
type SpendConditions struct {
	TimeLock      BlockHeight
	NumSignatures uint64
	PublicKeys    []PublicKey
}

// A TransactionSignature signs a single input to a transaction to help fulfill
// the unlock conditions of the transaction. It points to an input, a
// particular public key, has a timelock, and also indicates which parts of the
// transaction have been signed.
type TransactionSignature struct {
	InputID        OutputID // the OutputID of the Input that this signature is addressing. Using the index has also been considered.
	PublicKeyIndex uint64
	TimeLock       BlockHeight
	CoveredFields  CoveredFields
	Signature      Signature
}

type CoveredFields struct {
	WholeTransaction bool
	ArbitraryData    bool
	MinerFees        []uint8 // each element indicates an index which is signed.
	Inputs           []uint8
	Outputs          []uint8
	Contracts        []uint8
	FileProofs       []uint8
	Signatures       []uint8
}

// A FileContract contains the information necessary to enforce that a host
// stores a file.
type FileContract struct {
	ContractFund       Currency
	FileMerkleRoot     Hash
	FileSize           uint64 // probably in bytes, which means the last element in the merkle tree may not be exactly 64 bytes.
	Start, End         BlockHeight
	ChallengeFrequency BlockHeight // size of window, one window at a time
	Tolerance          uint64      // number of missed proofs before triggering unsuccessful termination
	ValidProofPayout   Currency
	ValidProofAddress  CoinAddress
	MissedProofPayout  Currency
	MissedProofAddress CoinAddress
}

// A StorageProof contains the fields needed for a host to prove that they are
// still storing a file.
type StorageProof struct {
	ContractID ContractID
	Segment    [SegmentSize]byte
	HashSet    []Hash
}

// Block.ID() returns a hash of the block, which is used as the block
// identifier. Transactions are not included in the hash.
func (b *Block) ID() BlockID {
	return BlockID(HashBytes(MarshalAll(
		b.ParentBlock,
		b.Timestamp,
		b.Nonce,
		b.MinerAddress,
		b.MerkleRoot,
	)))
}

// SubisdyID() returns the id of the output created by the block subsidy.
func (b *Block) SubsidyID() OutputID {
	bid := b.ID()
	return OutputID(HashBytes(append(bid[:], []byte("blockreward")...)))
}

// SigHash returns the hash of a transaction for a specific index.
// The index determines which TransactionSignature is included in the hash.
func (t *Transaction) SigHash(i int) Hash {
	if t.Signatures[i].CoveredFields.WholeTransaction {
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

	var signedData []byte
	if t.Signatures[i].CoveredFields.ArbitraryData {
		signedData = append(signedData, Marshal(t.ArbitraryData)...)
	}
	for _, minerFee := range t.Signatures[i].CoveredFields.MinerFees {
		signedData = append(signedData, Marshal(minerFee)...)
	}
	for _, input := range t.Signatures[i].CoveredFields.Inputs {
		signedData = append(signedData, Marshal(input)...)
	}
	for _, output := range t.Signatures[i].CoveredFields.Outputs {
		signedData = append(signedData, Marshal(output)...)
	}
	for _, contract := range t.Signatures[i].CoveredFields.Contracts {
		signedData = append(signedData, Marshal(contract)...)
	}
	for _, fileProof := range t.Signatures[i].CoveredFields.FileProofs {
		signedData = append(signedData, Marshal(fileProof)...)
	}
	for _, sig := range t.Signatures[i].CoveredFields.Signatures {
		signedData = append(signedData, Marshal(sig)...)
	}

	return HashBytes(signedData)
}

// Transaction.OuptutID() takes the index of the output and returns the
// output's ID.
func (t *Transaction) OutputID(index int) OutputID {
	return OutputID(HashBytes(append(Marshal(t), append([]byte("coinsend"), Marshal(uint64(index))...)...)))
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

// Transaction.fileContractID returns the id of a file contract given the index of the contract.
func (t *Transaction) FileContractID(index int) ContractID {
	return ContractID(HashBytes(append(Marshal(t), append([]byte("contract"), Marshal(uint64(index))...)...)))
}

// WindowIndex returns the index of the challenge window that is
// open during block height 'height'.
func (fc *FileContract) WindowIndex(height BlockHeight) (windowIndex BlockHeight, err error) {
	if height < fc.Start {
		err = errors.New("height below start point")
		return
	}
	if height >= fc.End {
		err = errors.New("height above end point")
	}

	windowIndex = (height - fc.Start) / fc.ChallengeFrequency
	return
}

// StorageProofOutput() returns the OutputID of the output created
// during the window index that was active at height 'height'.
func (fc *FileContract) StorageProofOutputID(fcID ContractID, height BlockHeight, proofValid bool) (outputID OutputID, err error) {
	proofString := proofString(proofValid)
	windowIndex, err := fc.WindowIndex(height)
	if err != nil {
		return
	}

	outputID = OutputID(HashBytes(append(fcID[:], append(proofString, Marshal(windowIndex)...)...)))
	return
}

// ContractTerminationOutputID() returns the ID of a contract termination
// output, given the id of the contract and the status of the termination.
func (fc *FileContract) ContractTerminationOutputID(fcID ContractID, successfulTermination bool) OutputID {
	terminationString := terminationString(successfulTermination)
	return OutputID(HashBytes(append(fcID[:], terminationString...)))
}

// Signature.MarshalSia implements the Marshaler interface for Signatures.
func (s *Signature) MarshalSia() []byte {
	if s.R == nil || s.S == nil {
		return []byte{0, 0}
	}
	// pretend Signature is a tuple of []bytes
	// this lets us use Marshal instead of doing manual length-prefixing
	return Marshal(struct{ R, S []byte }{s.R.Bytes(), s.S.Bytes()})
}

// Signature.UnmarshalSia implements the Unmarshaler interface for Signatures.
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

// PublicKey.MarshalSia implements the Marshaler interface for PublicKeys.
func (pk PublicKey) MarshalSia() []byte {
	if pk.X == nil || pk.Y == nil {
		return []byte{0, 0}
	}
	// see Signature.MarshalSia
	return Marshal(struct{ X, Y []byte }{pk.X.Bytes(), pk.Y.Bytes()})
}

// PublicKey.UnmarshalSia implements the Unmarshaler interface for PublicKeys.
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
