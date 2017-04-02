package types

import (
	"io"

	"github.com/NebulousLabs/Sia/encoding"
)

// MarshalSia implements the encoding.SiaMarshaler interface.
func (t Transaction) MarshalSia(w io.Writer) error {
	enc := encoding.NewEncoder(w)
	encoding.WriteInt(w, len((t.SiacoinInputs)))
	for i := range t.SiacoinInputs {
		t.SiacoinInputs[i].MarshalSia(w)
	}
	encoding.WriteInt(w, len((t.SiacoinOutputs)))
	for i := range t.SiacoinOutputs {
		t.SiacoinOutputs[i].MarshalSia(w)
	}
	encoding.WriteInt(w, len((t.FileContracts)))
	for i := range t.FileContracts {
		enc.Encode(t.FileContracts[i])
	}
	encoding.WriteInt(w, len((t.FileContractRevisions)))
	for i := range t.FileContractRevisions {
		enc.Encode(t.FileContractRevisions[i])
	}
	encoding.WriteInt(w, len((t.StorageProofs)))
	for i := range t.StorageProofs {
		enc.Encode(t.StorageProofs[i])
	}
	encoding.WriteInt(w, len((t.SiafundInputs)))
	for i := range t.SiafundInputs {
		enc.Encode(t.SiafundInputs[i])
	}
	encoding.WriteInt(w, len((t.SiafundOutputs)))
	for i := range t.SiafundOutputs {
		t.SiafundOutputs[i].MarshalSia(w)
	}
	encoding.WriteInt(w, len((t.MinerFees)))
	for i := range t.MinerFees {
		t.MinerFees[i].MarshalSia(w)
	}
	encoding.WriteInt(w, len((t.ArbitraryData)))
	for i := range t.ArbitraryData {
		encoding.WritePrefix(w, t.ArbitraryData[i])
	}
	encoding.WriteInt(w, len((t.TransactionSignatures)))
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
	for _, f := range fields {
		encoding.WriteInt(w, len(f))
		for _, u := range f {
			if err := encoding.WriteUint64(w, u); err != nil {
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
	encoding.WriteUint64(w, ts.PublicKeyIndex)
	encoding.WriteUint64(w, uint64(ts.Timelock))
	ts.CoveredFields.MarshalSia(w)
	return encoding.WritePrefix(w, ts.Signature)
}

// MarshalSia implements the encoding.SiaMarshaler interface.
func (uc UnlockConditions) MarshalSia(w io.Writer) error {
	encoding.WriteUint64(w, uint64(uc.Timelock))
	encoding.WriteInt(w, len(uc.PublicKeys))
	for _, spk := range uc.PublicKeys {
		spk.MarshalSia(w)
	}
	return encoding.WriteUint64(w, uc.SignaturesRequired)
}
