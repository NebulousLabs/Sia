package siacore

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/signatures"
)

// Though these are variables, they should never be changed during runtime.
// They get altered during testing.
var (
	BlockFrequency  = Timestamp(45)          // In seconds.
	TargetWindow    = BlockHeight(40)        // Number of blocks to use when calculating the target.
	FutureThreshold = Timestamp(3 * 60 * 60) // Seconds into the future block timestamps are valid.
	RootTarget      = Target{0, 1}

	MaxAdjustmentUp   = big.NewRat(103, 100)
	MaxAdjustmentDown = big.NewRat(97, 100)

	InitialCoinbase = Currency(300000)
	MinimumCoinbase = Currency(30000)

	GenesisAddress   = CoinAddress{}         // TODO: NEED TO CREATE A HARDCODED ADDRESS.
	GenesisTimestamp = Timestamp(1417070299) // Approx. 1:47pm EST Nov. 13th, 2014

	DEBUG = false
)

type (
	Timestamp   int64
	BlockHeight uint64
	Currency    uint64

	BlockID       hash.Hash
	OutputID      hash.Hash // An output id points to a specific output.
	ContractID    hash.Hash
	TransactionID hash.Hash
	CoinAddress   hash.Hash // An address is the hash of the spend conditions that unlock the output.
	Target        hash.Hash
)

// Eventually, the Block and the block header will be two separate structs.
// This will be put into practice when we implement merged mining.
type Block struct {
	ParentBlockID BlockID
	Timestamp     Timestamp
	Nonce         uint64
	MinerAddress  CoinAddress
	MerkleRoot    hash.Hash
	Transactions  []Transaction
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
	PublicKeys    []signatures.PublicKey
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
	Signature      signatures.Signature
}

type CoveredFields struct {
	WholeTransaction bool
	ArbitraryData    bool
	MinerFees        []uint8 // each element indicates an index which is signed.
	Inputs           []uint8
	Outputs          []uint8
	Contracts        []uint8
	StorageProofs    []uint8
	Signatures       []uint8
}

// A StorageProof contains the fields needed for a host to prove that they are
// still storing a file.
type StorageProof struct {
	ContractID ContractID
	Segment    [hash.SegmentSize]byte
	HashSet    []hash.Hash
}

// A FileContract contains the information necessary to enforce that a host
// stores a file.
type FileContract struct {
	ContractFund       Currency
	FileMerkleRoot     hash.Hash
	FileSize           uint64 // probably in bytes, which means the last element in the merkle tree may not be exactly 64 bytes.
	Start, End         BlockHeight
	ChallengeFrequency BlockHeight // size of window, one window at a time
	Tolerance          uint64      // number of missed proofs before triggering unsuccessful termination
	ValidProofPayout   Currency
	ValidProofAddress  CoinAddress
	MissedProofPayout  Currency
	MissedProofAddress CoinAddress
}

// CalculateCoinbase takes a height and from that derives the coinbase.
func CalculateCoinbase(height BlockHeight) Currency {
	if Currency(height) >= InitialCoinbase-MinimumCoinbase {
		return MinimumCoinbase
	} else {
		return InitialCoinbase - Currency(height)
	}
}

// Int returns a Target as a big.Int.
func (t Target) Int() *big.Int {
	return new(big.Int).SetBytes(t[:])
}

// Rat returns a Target as a big.Rat.
func (t Target) Rat() *big.Rat {
	return new(big.Rat).SetInt(t.Int())
}

// Inv returns the inverse of a Target as a big.Rat
func (t Target) Inverse() *big.Rat {
	r := t.Rat()
	return r.Inv(r)
}

// IntToTarget converts a big.Int to a Target.
func IntToTarget(i *big.Int) (t Target) {
	// i may overflow the maximum target.
	// In the event of overflow, return the maximum.
	if i.BitLen() > 256 {
		copy(t[:], bytes.Repeat([]byte{0xFF}, 32))
		return
	}
	b := i.Bytes()
	// need to preserve big-endianness
	offset := hash.HashSize - len(b)
	copy(t[offset:], b)
	return
}

// RatToTarget converts a big.Rat to a Target.
func RatToTarget(r *big.Rat) Target {
	// convert to big.Int to truncate decimal
	i := new(big.Int).Div(r.Num(), r.Denom())
	return IntToTarget(i)
}

// Block.ID() returns a hash of the block, which is used as the block
// identifier. Transactions are not included in the hash.
func (b Block) ID() BlockID {
	return BlockID(hash.HashBytes(encoding.MarshalAll(
		b.ParentBlockID,
		b.Timestamp,
		b.Nonce,
		b.MinerAddress,
		b.MerkleRoot,
	)))
}

// CheckTarget() returns true if the block id is lower than the target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// ExpectedTransactionMerkleRoot() returns the expected transaction
// merkle root of the block.
func (b Block) TransactionMerkleRoot() hash.Hash {
	var transactionHashes []hash.Hash
	for _, transaction := range b.Transactions {
		transactionHashes = append(transactionHashes, hash.HashBytes(encoding.Marshal(transaction)))
	}
	return hash.MerkleRoot(transactionHashes)
}

// SubisdyID() returns the id of the output created by the block subsidy.
func (b Block) SubsidyID() OutputID {
	bid := b.ID()
	return OutputID(hash.HashBytes(append(bid[:], "blockreward"...)))
}

// SigHash returns the hash of a transaction for a specific index.
// The index determines which TransactionSignature is included in the hash.
func (t *Transaction) SigHash(i int) hash.Hash {
	if t.Signatures[i].CoveredFields.WholeTransaction {
		return hash.HashBytes(encoding.MarshalAll(
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
		signedData = append(signedData, encoding.Marshal(t.ArbitraryData)...)
	}
	for _, minerFee := range t.Signatures[i].CoveredFields.MinerFees {
		signedData = append(signedData, encoding.Marshal(t.MinerFees[minerFee])...)
	}
	for _, input := range t.Signatures[i].CoveredFields.Inputs {
		signedData = append(signedData, encoding.Marshal(t.Inputs[input])...)
	}
	for _, output := range t.Signatures[i].CoveredFields.Outputs {
		signedData = append(signedData, encoding.Marshal(t.Outputs[output])...)
	}
	for _, contract := range t.Signatures[i].CoveredFields.Contracts {
		signedData = append(signedData, encoding.Marshal(t.FileContracts[contract])...)
	}
	for _, storageProof := range t.Signatures[i].CoveredFields.StorageProofs {
		signedData = append(signedData, encoding.Marshal(t.StorageProofs[storageProof])...)
	}
	for _, sig := range t.Signatures[i].CoveredFields.Signatures {
		signedData = append(signedData, encoding.Marshal(t.Signatures[sig])...)
	}

	return hash.HashBytes(signedData)
}

// Transaction.OuptutID() takes the index of the output and returns the
// output's ID.
func (t Transaction) OutputID(index int) OutputID {
	return OutputID(hash.HashAll(
		encoding.Marshal(t),
		[]byte("coinsend"),
		encoding.Marshal(index),
	))
}

// SpendConditions.CoinAddress() calculates the root hash of a merkle tree of the
// SpendConditions object, using the timelock, number of signatures required,
// and each public key as leaves.
func (sc *SpendConditions) CoinAddress() CoinAddress {
	tlHash := hash.HashObject(sc.TimeLock)
	nsHash := hash.HashObject(sc.NumSignatures)
	pkHashes := make([]hash.Hash, len(sc.PublicKeys))
	for i := range sc.PublicKeys {
		pkHashes[i] = hash.HashObject(sc.PublicKeys[i])
	}
	leaves := append([]hash.Hash{tlHash, nsHash}, pkHashes...)
	return CoinAddress(hash.MerkleRoot(leaves))
}

// Transaction.fileContractID returns the id of a file contract given the index of the contract.
func (t Transaction) FileContractID(index int) ContractID {
	return ContractID(hash.HashAll(
		encoding.Marshal(t),
		[]byte("contract"),
		encoding.Marshal(index),
	))
}

// WindowIndex returns the index of the challenge window that is
// open during block height 'height'.
func (fc *FileContract) WindowIndex(height BlockHeight) (windowIndex BlockHeight, err error) {
	if height < fc.Start {
		err = errors.New("height below start point")
		return
	} else if height >= fc.End {
		err = errors.New("height above end point")
		return
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

	outputID = OutputID(hash.HashAll(
		fcID[:],
		proofString,
		encoding.Marshal(windowIndex),
	))
	return
}

// ContractTerminationOutputID() returns the ID of a contract termination
// output, given the id of the contract and the status of the termination.
func ContractTerminationOutputID(fcID ContractID, successfulTermination bool) OutputID {
	return OutputID(hash.HashAll(
		fcID[:],
		terminationString(successfulTermination),
	))
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
