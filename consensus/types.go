package consensus

import (
	"bytes"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
)

type (
	Timestamp   int64
	BlockHeight uint64
	Currency    uint64

	BlockID     hash.Hash
	OutputID    hash.Hash
	ContractID  hash.Hash
	CoinAddress hash.Hash // The hash of the spend conditions of an output.
	Target      hash.Hash
)

type Block struct {
	ParentBlockID BlockID
	Timestamp     Timestamp
	Nonce         uint64
	MinerAddress  CoinAddress
	Transactions  []Transaction
}

// A Transaction is an update to the state of the network, can move money
// around, make contracts, etc.
type Transaction struct {
	Inputs        []Input
	MinerFees     []Currency
	Outputs       []Output
	FileContracts []FileContract
	StorageProofs []StorageProof
	ArbitraryData []string
	Signatures    []TransactionSignature
}

// An Input contains the ID of the output it's trying to spend, and the spend
// conditions that unlock the output.
type Input struct {
	OutputID        OutputID // the source of coins for the input
	SpendConditions SpendConditions
}

// SpendConditions is a timelock and a set of public keys that are used to
// unlock ouptuts.
type SpendConditions struct {
	TimeLock      BlockHeight
	NumSignatures uint64
	PublicKeys    []crypto.PublicKey
}

// An Output contains a volume of currency and a 'CoinAddress', which is just a
// hash of the spend conditions which unlock the output.
type Output struct {
	Value     Currency // how many coins are in the output
	SpendHash CoinAddress
}

// A FileContract contains the information necessary to enforce that a host
// stores a file.
type FileContract struct {
	FileMerkleRoot     hash.Hash
	FileSize           uint64 // in bytes
	Start, End         BlockHeight
	Payout             Currency
	ValidProofAddress  CoinAddress
	MissedProofAddress CoinAddress
}

// A StorageProof contains the fields needed for a host to prove that they are
// still storing a file. Though WindowIndex is of type BlockHeight, it refers
// to the index of the window, and not the height at which the window starts.
type StorageProof struct {
	ContractID ContractID
	Segment    [hash.SegmentSize]byte
	HashSet    []hash.Hash
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
	Signature      crypto.Signature
}

type CoveredFields struct {
	WholeTransaction bool
	Inputs           []uint64 // each element indicates an index which is signed.
	MinerFees        []uint64
	Outputs          []uint64
	Contracts        []uint64
	StorageProofs    []uint64
	ArbitraryData    []uint64
	Signatures       []uint64
}

// CalculateCoinbase takes a height and from that derives the coinbase.
func CalculateCoinbase(height BlockHeight) Currency {
	if Currency(height) >= InitialCoinbase-MinimumCoinbase {
		return MinimumCoinbase * 100000
	} else {
		return (InitialCoinbase - Currency(height)) * 100000
	}
}

// Block.ID() returns a hash of the block, which is used as the block
// identifier. Transactions are not included in the hash.
func (b Block) ID() BlockID {
	return BlockID(hash.HashBytes(encoding.MarshalAll(
		b.ParentBlockID,
		b.Nonce,
		b.MerkleRoot(),
	)))
}

// CheckTarget() returns true if the block id is lower than the target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// ExpectedTransactionMerkleRoot() returns the expected transaction
// merkle root of the block.
func (b Block) MerkleRoot() hash.Hash {
	var hashes []hash.Hash
	hashes = append(hashes, hash.HashObject(b.Timestamp))
	hashes = append(hashes, hash.HashObject(b.MinerAddress))
	for _, txn := range b.Transactions {
		hashes = append(hashes, hash.HashObject(txn))
	}
	return hash.MerkleRoot(hashes)
}

// SubisdyID() returns the id of the output created by the block subsidy.
func (b Block) SubsidyID() OutputID {
	bid := b.ID()
	return OutputID(hash.HashBytes(append(bid[:], "blockreward"...)))
}

// FileContractID returns the id of a file contract given the index of the contract.
//
// TODO: Reconsider how file contract ids are derived
func (t Transaction) FileContractID(index int) ContractID {
	return ContractID(hash.HashAll(
		encoding.Marshal(t.Outputs[0]),
		encoding.Marshal(t.FileContracts[index]),
		[]byte("contract"),
		encoding.Marshal(index),
	))
}

// OuptutID takes the index of the output and returns the output's ID.
//
// TODO: ID should not include the signatures.
func (t Transaction) OutputID(index int) OutputID {
	return OutputID(hash.HashAll(
		encoding.Marshal(t),
		[]byte("coinsend"),
		encoding.Marshal(index),
	))
}

// OutputSum returns the sum of all the outputs in the transaction, which must
// match the sum of all the inputs. Outputs created by storage proofs are not
// considered, as they were already considered when the contract was created.
func (t Transaction) OutputSum() (sum Currency) {
	// Add the miner fees.
	for _, fee := range t.MinerFees {
		sum += fee
	}

	// Add the contract payouts
	for _, contract := range t.FileContracts {
		sum += contract.Payout
	}

	// Add the outputs
	for _, output := range t.Outputs {
		sum += output.Value
	}

	return
}

// SigHash returns the hash of a transaction for a specific index.  The index
// determines which TransactionSignature is included in the hash.
func (t Transaction) SigHash(i int) hash.Hash {
	var signedData []byte
	if t.Signatures[i].CoveredFields.WholeTransaction {
		signedData = append(signedData, encoding.MarshalAll(
			t.Inputs,
			t.MinerFees,
			t.Outputs,
			t.FileContracts,
			t.StorageProofs,
			t.ArbitraryData,
			t.Signatures[i].InputID,
			t.Signatures[i].PublicKeyIndex,
			t.Signatures[i].TimeLock,
		)...)
	} else {
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
		for _, arbData := range t.Signatures[i].CoveredFields.ArbitraryData {
			signedData = append(signedData, encoding.Marshal(t.ArbitraryData[arbData])...)
		}
	}

	for _, sig := range t.Signatures[i].CoveredFields.Signatures {
		signedData = append(signedData, encoding.Marshal(t.Signatures[sig])...)
	}

	return hash.HashBytes(signedData)
}

// StorageProofOutputID returns the OutputID of the output created during the
// window index that was active at height 'height'.
func (fcID ContractID) StorageProofOutputID(proofValid bool) (outputID OutputID) {
	proofString := proofString(proofValid)
	outputID = OutputID(hash.HashAll(
		fcID[:],
		proofString,
	))
	return
}

// CoinAddress calculates the root hash of a merkle tree of the SpendConditions
// object, using the timelock, number of signatures required, and each public
// key as leaves.
func (sc SpendConditions) CoinAddress() CoinAddress {
	tlHash := hash.HashObject(sc.TimeLock)
	nsHash := hash.HashObject(sc.NumSignatures)
	pkHashes := make([]hash.Hash, len(sc.PublicKeys))
	for i := range sc.PublicKeys {
		pkHashes[i] = hash.HashObject(sc.PublicKeys[i])
	}
	leaves := append([]hash.Hash{tlHash, nsHash}, pkHashes...)
	return CoinAddress(hash.MerkleRoot(leaves))
}

// proofString returns the string to be used when generating the output id of a
// valid proof if bool is set to true, and it returns the string to be used in
// a missed proof if the bool is set to false.
func proofString(proofValid bool) []byte {
	if proofValid {
		return []byte("validproof")
	} else {
		return []byte("missedproof")
	}
}
