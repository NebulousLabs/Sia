package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// The header for all siag files. Do not change.
	SiagFileHeader    = "siag"
	SiagFileExtension = ".siakey"
	SiagFileVersion   = "1.0"
)

var (
	ErrInconsistentKeys     = errors.New("keyfiles provided that are for different addresses")
	ErrInsufficientKeys     = errors.New("not enough keys provided to spend the siafunds")
	ErrInsufficientSiafunds = errors.New("not enough siafunds in the keys provided to complete transction")
	ErrNoKeyfile            = errors.New("no keyfile has been presented")
	ErrUnknownHeader        = errors.New("file contains the wrong header")
	ErrUnknownVersion       = errors.New("file has an unknown version number")
)

// A SiagKeyPair is the keypair used by siag.
type SiagKeyPair struct {
	Header           string
	Version          string
	Index            int // should be uint64 - too late now
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
}

// AddSiagSiafundAddress loads a siafund address from a siag key. The private
// key is NOT loaded.
func (w *Wallet) AddSiagSiafundAddress(keyfile string) error {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	var skp SiagKeyPair
	err := encoding.ReadFile(keyfile, &skp)
	if err != nil {
		return err
	}
	if skp.Header != SiagFileHeader {
		return ErrUnknownHeader
	}
	if skp.Version != SiagFileVersion {
		return ErrUnknownVersion
	}
	w.siafundAddresses[skp.UnlockConditions.UnlockHash()] = struct{}{}
	// Janky println... but it's important to get this message to the user.
	println("Loaded a siafund address. Please note that the private key was not loaded, you must KEEP the original keyfile. You must restart said before your balance can be displayed.")
	w.save()
	return nil
}

// SpendSiagSiafunds sends siafunds to another address. The siacoins stored in
// the siafunds are sent to an address in the wallet.
func (w *Wallet) SpendSiagSiafunds(amount types.Currency, dest types.UnlockHash, keyfiles []string) (txn types.Transaction, err error) {
	if len(keyfiles) < 1 {
		return types.Transaction{}, ErrNoKeyfile
	}

	// Load the siafund keys and verify they are sufficient to sign the
	// transaction.
	var skps []SiagKeyPair
	for i, keyfile := range keyfiles {
		err = encoding.ReadFile(keyfile, &skps[i])
		if err != nil {
			return types.Transaction{}, err
		}

		if skps[i].Header != SiagFileHeader {
			return types.Transaction{}, ErrUnknownHeader
		}
		if skps[i].Version != SiagFileVersion {
			return types.Transaction{}, ErrUnknownVersion
		}
	}

	// Check that all of the loaded files have the same address, and that there
	// are enough to create the transaction.
	baseUnlockHash := skps[0].UnlockConditions.UnlockHash()
	for _, skp := range skps {
		if skp.UnlockConditions.UnlockHash() != baseUnlockHash {
			return types.Transaction{}, ErrInconsistentKeys
		}
	}
	if uint64(len(skps)) < skps[0].UnlockConditions.SignaturesRequired {
		return types.Transaction{}, ErrInsufficientKeys
	}

	// Check that there are enough siafunds in the key to complete the spend.
	lockID := w.mu.RLock()
	var availableSiafunds types.Currency
	var sfoids []types.SiafundOutputID
	for sfoid, sfo := range w.siafundOutputs {
		if sfo.UnlockHash == baseUnlockHash {
			availableSiafunds = availableSiafunds.Add(sfo.Value)
			sfoids = append(sfoids, sfoid)
		}
		if availableSiafunds.Cmp(amount) >= 0 {
			break
		}
	}
	w.mu.RUnlock(lockID)
	if availableSiafunds.Cmp(amount) < 0 {
		return types.Transaction{}, ErrInsufficientSiafunds
	}

	// Truncate the keys to exactly the number needed.
	skps = skps[:skps[0].UnlockConditions.SignaturesRequired]

	// Assemble the base transction, including a 10 siacoin fee if possible.
	id, err := w.RegisterTransaction(txn)
	if err != nil {
		return types.Transaction{}, err
	}
	// Add a miner fee - if funding the transaction fails, we'll just send a
	// transaction with no fee.
	_, err = w.FundTransaction(id, types.NewCurrency64(TransactionFee))
	if err == nil {
		_, _, err = w.AddMinerFee(id, types.NewCurrency64(TransactionFee))
		if err != nil {
			return types.Transaction{}, err
		}
	}
	// Add the siafund inputs to the transcation.
	for _, sfoid := range sfoids {
		// Get an address for the siafund claims.
		lockID := w.mu.Lock()
		claimDest, _, err := w.coinAddress(false)
		w.mu.Unlock(lockID)
		if err != nil {
			return types.Transaction{}, err
		}

		// Assemble the SiafundInput to spend this output.
		sfi := types.SiafundInput{
			ParentID:         sfoid,
			UnlockConditions: skps[0].UnlockConditions,
			ClaimUnlockHash:  claimDest,
		}
		_, _, err = w.AddSiafundInput(id, sfi)
		if err != nil {
			return types.Transaction{}, err
		}
	}
	// Add the siafund output to the transaction.
	sfo := types.SiafundOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	_, _, err = w.AddSiafundOutput(id, sfo)
	if err != nil {
		return types.Transaction{}, err
	}
	// Add a refund siafund output if needed.
	if amount.Cmp(availableSiafunds) != 0 {
		refund := amount.Sub(availableSiafunds)
		sfo := types.SiafundOutput{
			Value:      refund,
			UnlockHash: baseUnlockHash,
		}
		_, _, err = w.AddSiafundOutput(id, sfo)
		if err != nil {
			return types.Transaction{}, err
		}
	}
	// Add signatures for the siafund inputs.
	sigIndex := 0
	for _, sfoid := range sfoids {
		for _, key := range skps {
			txnSig := types.TransactionSignature{
				ParentID:       crypto.Hash(sfoid),
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
				PublicKeyIndex: uint64(key.Index),
			}
			txn.TransactionSignatures = append(txn.TransactionSignatures, txnSig)
			sigHash := txn.SigHash(sigIndex)
			encodedSig, err := crypto.SignHash(sigHash, key.SecretKey)
			if err != nil {
				return types.Transaction{}, err
			}
			txn.TransactionSignatures[sigIndex].Signature = types.Signature(encodedSig[:])

			txn, _, err = w.AddTransactionSignature(id, txn.TransactionSignatures[sigIndex])
			if err != nil {
				return types.Transaction{}, err
			}

			sigIndex++
		}
	}

	// Sign the transaction.
	txn, err = w.SignTransaction(id, true)
	if err != nil {
		return types.Transaction{}, err
	}

	err = w.tpool.AcceptTransaction(txn)
	if err != nil {
		return types.Transaction{}, err
	}
	return txn, nil
}
