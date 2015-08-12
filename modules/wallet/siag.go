package wallet

/*
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

// SendSiagSiafunds sends siafunds to another address. The siacoins stored in
// the siafunds are sent to an address in the wallet.
func (w *Wallet) SendSiagSiafunds(amount types.Currency, dest types.UnlockHash, keyfiles []string) ([]types.Transaction, error) {
	if len(keyfiles) < 1 {
		return nil, ErrNoKeyfile
	}

	// Load the siafund keys and verify they are sufficient to sign the
	// transaction.
	skps := make([]SiagKeyPair, len(keyfiles))
	for i, keyfile := range keyfiles {
		err := encoding.ReadFile(keyfile, &skps[i])
		if err != nil {
			return nil, err
		}

		if skps[i].Header != SiagFileHeader {
			return nil, ErrUnknownHeader
		}
		if skps[i].Version != SiagFileVersion {
			return nil, ErrUnknownVersion
		}
	}

	// Check that all of the loaded files have the same address, and that there
	// are enough to create the transaction.
	baseUnlockHash := skps[0].UnlockConditions.UnlockHash()
	for _, skp := range skps {
		if skp.UnlockConditions.UnlockHash() != baseUnlockHash {
			return nil, ErrInconsistentKeys
		}
	}
	if uint64(len(skps)) < skps[0].UnlockConditions.SignaturesRequired {
		return nil, ErrInsufficientKeys
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
		return nil, ErrInsufficientSiafunds
	}

	// Truncate the keys to exactly the number needed.
	skps = skps[:skps[0].UnlockConditions.SignaturesRequired]

	// Register the transaction and add a fee, if possible.
	txnBuilder := w.StartTransaction()
	err := txnBuilder.FundSiacoins(types.NewCurrency64(TransactionFee))
	if err == nil {
		txnBuilder.AddMinerFee(types.NewCurrency64(TransactionFee))
	}

	// Add the siafund inputs to the transcation.
	for _, sfoid := range sfoids {
		// Get an address for the siafund claims.
		lockID := w.mu.Lock()
		claimDest, _, err := w.coinAddress(false)
		w.mu.Unlock(lockID)
		if err != nil {
			return nil, err
		}

		// Assemble the SiafundInput to spend this output.
		sfi := types.SiafundInput{
			ParentID:         sfoid,
			UnlockConditions: skps[0].UnlockConditions,
			ClaimUnlockHash:  claimDest,
		}
		txnBuilder.AddSiafundInput(sfi)
	}

	// Add the siafund output to the transaction.
	sfo := types.SiafundOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	txnBuilder.AddSiafundOutput(sfo)
	// Add a refund siafund output if needed.
	if amount.Cmp(availableSiafunds) != 0 {
		refund := availableSiafunds.Sub(amount)
		sfo := types.SiafundOutput{
			Value:      refund,
			UnlockHash: baseUnlockHash,
		}
		txnBuilder.AddSiafundOutput(sfo)
	}

	// Add signatures for the siafund inputs.
	sigIndex := 0
	txn, _ := txnBuilder.View()
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
				return nil, err
			}
			txn.TransactionSignatures[sigIndex].Signature = encodedSig[:]
			txnBuilder.AddTransactionSignature(txn.TransactionSignatures[sigIndex])
			sigIndex++
		}
	}

	// Sign the transaction.
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return nil, err
	}
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return nil, err
	}
	return txnSet, nil
}

// WatchSiagSiafundAddress loads a siafund address from a siag key. The private
// key is NOT loaded.
func (w *Wallet) WatchSiagSiafundAddress(keyfile string) error {
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
	println("Loaded a siafund address. Please note that the private key was not loaded, you must KEEP the original keyfile. You must restart siad before your balance can be displayed.")
	w.save()
	return nil
}
*/
