package sia

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func EncUint64(i uint64) (b []byte) {
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, i)
	return
}

func MarshalAll(data ...interface{}) []byte {
	buf := new(bytes.Buffer)
	var enc []byte
	for i := range data {
		switch d := data[i].(type) {
		case []byte:
			enc = d
		case string:
			enc = []byte(d)
		case uint64:
			enc = EncUint64(d)
		case Hash:
			enc = d[:]
		// more to come
		default:
			panic(fmt.Sprintf("can't marshal type %T", d))
		}
		buf.Write(enc)
	}
	return buf.Bytes()
}

func (s *Signature) Bytes() []byte {
	return []byte(s.R.String() + s.S.String())
}

func (pk *PublicKey) Bytes() []byte {
	return []byte(pk.X.String() + pk.Y.String())
}

func (txn *Transaction) Bytes() []byte {
	b := new(bytes.Buffer)
	b.Write(EncUint64(uint64(txn.Version)))
	b.Write(txn.ArbitraryData)
	b.Write(EncUint64(uint64(txn.MinerFee)))
	// Inputs
	b.WriteByte(uint8(len(txn.Inputs)))
	for i := range txn.Inputs {
		b.Write(txn.Inputs[i].Bytes())
	}
	// Ouputs
	b.WriteByte(uint8(len(txn.Outputs)))
	for i := range txn.Outputs {
		b.Write(txn.Outputs[i].Bytes())
	}
	// File Contracts
	b.WriteByte(uint8(len(txn.FileContracts)))
	for i := range txn.FileContracts {
		b.Write(txn.FileContracts[i].Bytes())
	}
	// Storage Proofs
	b.WriteByte(uint8(len(txn.StorageProofs)))
	for i := range txn.StorageProofs {
		b.Write(txn.StorageProofs[i].Bytes())
	}
	// Signatures
	b.WriteByte(uint8(len(txn.Signatures)))
	for i := range txn.Signatures {
		b.Write(txn.Signatures[i].Bytes())
	}
	return b.Bytes()
}

func (i *Input) Bytes() []byte {
	return append(i.OutputID[:], i.SpendConditions.Bytes()...)
}

func (o *Output) Bytes() []byte {
	return append(EncUint64(uint64(o.Value)), o.SpendHash[:]...)
}

func (sc *SpendConditions) Bytes() []byte {
	b := new(bytes.Buffer)
	b.Write(EncUint64(uint64(sc.TimeLock)))
	b.Write(EncUint64(uint64(sc.NumSignatures)))
	b.WriteByte(uint8(len(sc.PublicKeys)))
	for i := range sc.PublicKeys {
		b.Write(sc.PublicKeys[i].Bytes())
	}
	return b.Bytes()
}

func (sc *SpendConditions) MerkleRoot() Hash {
	tlHash := HashBytes(EncUint64(uint64(sc.TimeLock)))
	nsHash := HashBytes(EncUint64(uint64(sc.NumSignatures)))
	pkHashes := make([]Hash, len(sc.PublicKeys))
	for i := range sc.PublicKeys {
		pkHashes[i] = HashBytes(sc.PublicKeys[i].Bytes())
	}
	leaves := append([]Hash{tlHash, nsHash}, pkHashes...)
	return MerkleRoot(leaves)
}

func (ts *TransactionSignature) Bytes() []byte {
	b := new(bytes.Buffer)
	b.Write(ts.InputID[:])
	b.WriteByte(ts.PublicKeyIndex)
	b.Write(EncUint64(uint64(ts.TimeLock)))
	b.Write(ts.CoveredFields.Bytes())
	b.Write(ts.Signature.Bytes())
	return b.Bytes()
}

func (cf *CoveredFields) Bytes() []byte {
	b := new(bytes.Buffer)
	var flags uint8
	if cf.Version {
		flags &= 1 << 0
	}
	if cf.ArbitraryData {
		flags &= 1 << 1
	}
	if cf.ArbitraryData {
		flags &= 1 << 2
	}
	b.WriteByte(flags)

	// Inputs
	b.WriteByte(uint8(len(cf.Inputs)))
	for i := range cf.Inputs {
		b.WriteByte(cf.Inputs[i])
	}
	// Outputs
	b.WriteByte(uint8(len(cf.Outputs)))
	for i := range cf.Outputs {
		b.WriteByte(cf.Outputs[i])
	}
	// Contracts
	b.WriteByte(uint8(len(cf.Contracts)))
	for i := range cf.Contracts {
		b.WriteByte(cf.Contracts[i])
	}
	// File Proofs
	b.WriteByte(uint8(len(cf.FileProofs)))
	for i := range cf.FileProofs {
		b.WriteByte(cf.FileProofs[i])
	}

	return b.Bytes()
}

func (fc *FileContract) Bytes() []byte {
	return MarshalAll(
		uint64(fc.ContractFund),
		fc.FileMerkleRoot,
		fc.FileSize,
		uint64(fc.Start), uint64(fc.End),
		uint64(fc.ChallengeFrequency),
		uint64(fc.Tolerance),
		uint64(fc.ValidProofPayout),
		Hash(fc.ValidProofAddress),
		uint64(fc.MissedProofPayout),
		Hash(fc.MissedProofAddress),
		Hash(fc.SuccessAddress),
		Hash(fc.FailureAddress),
	)
}

func (sp *StorageProof) Bytes() []byte {
	b := new(bytes.Buffer)
	b.Write(sp.ContractID[:])
	b.Write(sp.Segment[:])
	b.WriteByte(uint8(len(sp.HashSet)))
	for i := range sp.HashSet {
		if sp.HashSet[i] == nil {
			b.WriteByte(0)
		} else {
			b.WriteByte(1)
			b.Write((*sp.HashSet[i])[:])
		}
	}
	return b.Bytes()
}
