package types

import (
	"encoding/binary"
	"io"

	"github.com/NebulousLabs/Sia/encoding"
)

// MarshalSia implements the encoding.SiaMarshaler interface.
func (t Transaction) MarshalSia(w io.Writer) error {
	length := make([]byte, 8)
	enc := encoding.NewEncoder(w)
	binary.LittleEndian.PutUint64(length, uint64(len((t.SiacoinInputs))))
	w.Write(length)
	for i := range t.SiacoinInputs {
		t.SiacoinInputs[i].MarshalSia(w)
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.SiacoinOutputs))))
	w.Write(length)
	for i := range t.SiacoinOutputs {
		t.SiacoinOutputs[i].MarshalSia(w)
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.FileContracts))))
	w.Write(length)
	for i := range t.FileContracts {
		enc.Encode(t.FileContracts[i])
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.FileContractRevisions))))
	w.Write(length)
	for i := range t.FileContractRevisions {
		enc.Encode(t.FileContractRevisions[i])
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.StorageProofs))))
	w.Write(length)
	for i := range t.StorageProofs {
		enc.Encode(t.StorageProofs[i])
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.SiafundInputs))))
	w.Write(length)
	for i := range t.SiafundInputs {
		enc.Encode(t.SiafundInputs[i])
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.SiafundOutputs))))
	w.Write(length)
	for i := range t.SiafundOutputs {
		t.SiafundOutputs[i].MarshalSia(w)
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.MinerFees))))
	w.Write(length)
	for i := range t.MinerFees {
		t.MinerFees[i].MarshalSia(w)
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.ArbitraryData))))
	w.Write(length)
	for i := range t.ArbitraryData {
		binary.LittleEndian.PutUint64(length, uint64(len((t.ArbitraryData[i]))))
		w.Write(length)
		w.Write(t.ArbitraryData[i])
	}
	binary.LittleEndian.PutUint64(length, uint64(len((t.TransactionSignatures))))
	w.Write(length)
	for i := range t.TransactionSignatures {
		err := t.TransactionSignatures[i].MarshalSia(w)
		if err != nil {
			return err
		}
	}
	return nil
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (sci SiacoinInput) MarshalSia(w io.Writer) error {
	w.Write(sci.ParentID[:])
	return sci.UnlockConditions.MarshalSia(w)
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (sco SiacoinOutput) MarshalSia(w io.Writer) error {
	sco.Value.MarshalSia(w)
	_, err := w.Write(sco.UnlockHash[:])
	return err
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (sfi SiafundInput) MarshalSia(w io.Writer) error {
	w.Write(sfi.ParentID[:])
	sfi.UnlockConditions.MarshalSia(w)
	_, err := w.Write(sfi.ClaimUnlockHash[:])
	return err
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (sfo SiafundOutput) MarshalSia(w io.Writer) error {
	sfo.Value.MarshalSia(w)
	w.Write(sfo.UnlockHash[:])
	return sfo.ClaimStart.MarshalSia(w)
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (cf CoveredFields) MarshalSia(w io.Writer) error {
	encoding.NewEncoder(w).Encode(cf.WholeTransaction)
	fields := [][]uint64{
		cf.SiacoinInputs,
		cf.SiacoinOutputs,
		cf.FileContracts,
		cf.FileContractRevisions,
		cf.StorageProofs,
		cf.SiafundInputs,
		cf.SiafundOutputs,
		cf.MinerFees,
		cf.ArbitraryData,
		cf.TransactionSignatures,
	}
	b := make([]byte, 8)
	for _, f := range fields {
		binary.LittleEndian.PutUint64(b, uint64(len(f)))
		w.Write(b)
		for _, u := range f {
			binary.LittleEndian.PutUint64(b, u)
			if _, err := w.Write(b); err != nil {
				return err
			}
		}
	}
	return nil
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (spk SiaPublicKey) MarshalSia(w io.Writer) error {
	w.Write(spk.Algorithm[:])
	return encoding.WritePrefix(w, spk.Key)
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (ts TransactionSignature) MarshalSia(w io.Writer) error {
	w.Write(ts.ParentID[:])
	w.Write(encoding.EncUint64(ts.PublicKeyIndex))
	w.Write(encoding.EncUint64(uint64(ts.Timelock)))
	ts.CoveredFields.MarshalSia(w)
	return encoding.WritePrefix(w, ts.Signature)
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (uc UnlockConditions) MarshalSia(w io.Writer) error {
	w.Write(encoding.EncUint64(uint64(uc.Timelock)))
	w.Write(encoding.EncUint64(uint64(len(uc.PublicKeys))))
	for _, spk := range uc.PublicKeys {
		spk.MarshalSia(w)
	}
	_, err := w.Write(encoding.EncUint64(uc.SignaturesRequired))
	return err
}
