package consensus

// TODO: Swtich to 128 bit Currency, which is overflow-safe. Then update
// CalculateCoinbase.

// TODO: Enforce the 100 block spending hold on certain types of outputs: Miner
// payouts, storage proof outputs, siafund claims.

// TODO: Enforce restrictions on which storage proof transactions are legal

// TODO: Enforce siafund rules in consensus.

import (
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
	CoinAddress hash.Hash
	Target      hash.Hash
)

var ZeroAddress = CoinAddress{0}

// A Block contains all of the changes to the state that have occurred since
// the previous block. There are constraints that make it difficult and
// rewarding to find a block.
type Block struct {
	ParentID     BlockID
	Nonce        uint64
	Timestamp    Timestamp
	MinerPayouts []Output
	Transactions []Transaction
}

// A Transaction is an update to the state of the network, can move money
// around, make contracts, etc.
type Transaction struct {
	Inputs         []Input
	MinerFees      []Currency
	Outputs        []Output
	FileContracts  []FileContract
	StorageProofs  []StorageProof
	SiafundInputs  []SiafundInput
	SiafundOutputs []SiafundOutput
	ArbitraryData  []string
	Signatures     []TransactionSignature
}

// An Input contains the ID of the output it's trying to spend, and the spend
// conditions that unlock the output.
type Input struct {
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
	FileSize           uint64
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

// The CoveredFields portion of a signature indicates which fields in the
// transaction have been covered by the signature. Each slice of elements in a
// transaction is represented by a slice of indices. The indicies must be
// sorted, must not repeat, and must point to elements that exist within the
// transaction. If 'WholeTransaction' is set to true, all other fields must be
// empty except for the Signatures field.
type CoveredFields struct {
	WholeTransaction bool
	Inputs           []uint64
	MinerFees        []uint64
	Outputs          []uint64
	FileContracts    []uint64
	StorageProofs    []uint64
	SiafundInputs    []uint64
	SiafundOutputs   []uint64
	ArbitraryData    []uint64
	Signatures       []uint64
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
		b.ParentID,
		b.Nonce,
		b.MerkleRoot(),
	)))
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
	return OutputID(hash.HashBytes(encoding.MarshalAll(b.ID(), i)))
}

// FileContractID returns the id of a file contract given the index of the
// contract. The id is derived by marshalling all of the fields in the
// transaction except for the signatures and then appending the string "file
// contract" and the index of the contract.
func (t Transaction) FileContractID(i int) ContractID {
	return ContractID(hash.HashBytes(encoding.MarshalAll(
		t.Inputs,
		t.MinerFees,
		t.Outputs,
		t.FileContracts,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.ArbitraryData,
		[8]byte{'f', 'i', 'l', 'e', 'c', 'o', 'u', 't'},
		i,
	)))
}

// OutputID gets the id of an output in the transaction, which is derived from
// marshalling all of the fields in the transaction except for the signatures
// and then appending the string "siacoin output" and the index of the output.
func (t Transaction) OutputID(i int) OutputID {
	return OutputID(hash.HashBytes(encoding.MarshalAll(
		t.Inputs,
		t.MinerFees,
		t.Outputs,
		t.FileContracts,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.ArbitraryData,
		[8]byte{'s', 'c', 'o', 'i', 'n', 'o', 'u', 't'},
		i,
	)))
}

// StorageProofOutputID returns the OutputID of the output created during the
// window index that was active at height 'height'.
func (fcID ContractID) StorageProofOutputID(proofValid bool) (outputID OutputID) {
	outputID = OutputID(hash.HashBytes(encoding.MarshalAll(
		fcID,
		proofValid,
	)))
	return
}

// SiafundOutputID returns the id of the siafund output that was specified and
// index `i` in the transaction.
func (t Transaction) SiafundOutputID(i int) OutputID {
	return OutputID(hash.HashBytes(encoding.MarshalAll(
		t.Inputs,
		t.MinerFees,
		t.Outputs,
		t.FileContracts,
		t.StorageProofs,
		t.SiafundInputs,
		t.SiafundOutputs,
		t.ArbitraryData,
		[8]byte{'s', 'f', 'u', 'n', 'd', 'o', 'u', 't'},
		i,
	)))
}

// SiaClaimOutputID returns the id of the siacoin output that is created when
// the siafund output gets spent.
func (id OutputID) SiaClaimOutputID(i int) OutputID {
	return OutputID(hash.HashBytes(encoding.MarshalAll(
		id,
	)))
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
			t.Inputs,
			t.MinerFees,
			t.Outputs,
			t.FileContracts,
			t.StorageProofs,
			t.SiafundInputs,
			t.SiafundOutputs,
			t.ArbitraryData,
			t.Signatures[i].InputID,
			t.Signatures[i].PublicKeyIndex,
			t.Signatures[i].TimeLock,
		)
	} else {
		for _, minerFee := range cf.MinerFees {
			signedData = append(signedData, encoding.Marshal(t.MinerFees[minerFee])...)
		}
		for _, input := range cf.Inputs {
			signedData = append(signedData, encoding.Marshal(t.Inputs[input])...)
		}
		for _, output := range cf.Outputs {
			signedData = append(signedData, encoding.Marshal(t.Outputs[output])...)
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
