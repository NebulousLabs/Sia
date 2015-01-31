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
	Siafund     uint64

	BlockID     hash.Hash
	OutputID    hash.Hash
	ContractID  hash.Hash
	CoinAddress hash.Hash // The hash of the spend conditions of an output.
	Target      hash.Hash
)

// TODO: Swtich MinerAddress to a MinerPayout, and add rules to consensus that
// enforce the Value sum of the miner payout outputs is exactly equal to the
// block subsidy.
type Block struct {
	ParentBlockID BlockID
	Nonce         uint64
	Timestamp     Timestamp
	MinerAddress  CoinAddress
	// MinerPayout []Output
	Transactions []Transaction
}

// A Transaction is an update to the state of the network, can move money
// around, make contracts, etc.
//
// TODO: Enable siafund stuff
type Transaction struct {
	Inputs        []Input
	MinerFees     []Currency
	Outputs       []Output
	FileContracts []FileContract
	StorageProofs []StorageProof
	// SiafundInputs  []SiafundInput
	// SiafundOutputs []SiafundOutput
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
	PublicKeys    []crypto.PublicKey
	NumSignatures uint64
}

// An Output contains a volume of currency and a 'CoinAddress', which is just a
// hash of the spend conditions which unlock the output.
type Output struct {
	Value     Currency
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

// A storage proof contains a segment and the HashSet required to prove that
// the segment is a part of the data in the FileMerkleRoot of the FileContract
// that the storage proof fulfills.
type StorageProof struct {
	ContractID ContractID
	Segment    [hash.SegmentSize]byte
	HashSet    []hash.Hash
}

// A SiafundInput is close to a SiacoinInput, except that the asset being spent
// is a SiaFund.
type SiafundInput struct {
	OutputID        OutputID
	SpendConditions SpendConditions
}

// A SiafundOutput contains a value and a spend hash like the SiacoinOutput,
// but it also contians a ClaimDestination and a ClaimStart. The
// ClaimDestination is the address that will receive siacoins when the siafund
// output is spent. The ClaimStart will be comapred to the SiafundPool to
// figure out how many siacoins the ClaimDestination will receive.
type SiafundOutput struct {
	Value            Siafund
	SpendHash        CoinAddress
	ClaimDestination CoinAddress
	ClaimStart       Currency
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
	Signature      crypto.Signature
}

// TODO: If `WholeTransaction` is set to true, then all other fields except for
// Signatures should be empty, this should be a consensus rule.
//
// TODO: Signature can include each element at most once. If there are repeat
// elements, everything is invalid.
//
// TODO: We might, for speed reasons, want to also force the fields to be
// sorted already.
type CoveredFields struct {
	WholeTransaction bool
	Inputs           []uint64 // each element indicates an index which is signed.
	MinerFees        []uint64
	Outputs          []uint64
	Contracts        []uint64
	StorageProofs    []uint64
	// SiafundInputs []uint64
	// SiafundOutputs  []unit64
	ArbitraryData []uint64
	Signatures    []uint64
}

// CalculateCoinbase takes a height and from that derives the coinbase.
//
// TODO: Switch to a different constant because of using 128 bit values for the
// currency.
func CalculateCoinbase(height BlockHeight) Currency {
	if Currency(height) >= InitialCoinbase-MinimumCoinbase {
		return MinimumCoinbase * 100000
	} else {
		return (InitialCoinbase - Currency(height)) * 100000
	}
}

// ID returns the id of a block, which is calculated by concatenating the
// parent block id, the block nonce, and the block merkle root and taking the
// hash.
func (b Block) ID() BlockID {
	return BlockID(hash.HashBytes(encoding.MarshalAll(
		b.ParentBlockID,
		b.Nonce,
		b.MerkleRoot(),
	)))
}

// CheckTarget returns true if the block id is lower than the target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// MerkleRoot calculates the merkle root of the block. The leaves of the merkle
// tree are composed of the Timestamp, the set of miner outputs (one leaf), and
// all of the transactions (many leaves).
//
// TODO: change the miner address to the miner outputs.
func (b Block) MerkleRoot() hash.Hash {
	leaves := []hash.Hash{
		hash.HashObject(b.Timestamp),
		hash.HashObject(b.MinerAddress),
	}
	for _, txn := range b.Transactions {
		leaves = append(leaves, hash.HashObject(txn))
	}
	return hash.MerkleRoot(leaves)
}

// SubisdyID returns the id of the output created by the block subsidy.
//
// TODO: Adjust so that it returns the id of the miner outputs. Also reconsider
// how output ids are created.
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

// SigHash returns the hash of a transaction for a specific signature. `i` is
// the index of the signature for which the hash is being returned. If
// `WholeTransaction` is set to true for the siganture, then all of the
// transaction fields except the signatures are included in the transactions.
// If `WholeTransaction` is set to false, then the fees, inputs, ect. are all
// added individually. The signatures are added individually regardless of the
// value of `WholeTransaction`.
//
// TODO: add loops for the siafunds stuff
func (t Transaction) SigHash(i int) hash.Hash {
	var signedData []byte
	if t.Signatures[i].CoveredFields.WholeTransaction {
		signedData = encoding.MarshalAll(
			t.Inputs,
			t.MinerFees,
			t.Outputs,
			t.FileContracts,
			t.StorageProofs,
			// Siafunds
			// Stuff
			// Here
			t.ArbitraryData,
			t.Signatures[i].InputID,
			t.Signatures[i].PublicKeyIndex,
			t.Signatures[i].TimeLock,
		)
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
		// Siafunds
		// Stuff
		// Here
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
//
// TODO: Reconsider how the StorageProofOutputID is determined.
func (fcID ContractID) StorageProofOutputID(proofValid bool) (outputID OutputID) {
	proofString := proofString(proofValid)
	outputID = OutputID(hash.HashAll(
		fcID[:],
		proofString,
	))
	return
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
