package wallet

import (
	"bytes"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

type hashSigner func(crypto.Hash, crypto.SecretKey) (crypto.Signature, error)

type signatureAdder func(*types.Transaction, types.CoveredFields, types.UnlockConditions, crypto.Hash, spendableKey) error

// addSignatures will sign a transaction using a spendable key, with support
// for multisig spendable keys. Because of the restricted input, the function
// is compatible with both siacoin inputs and siafund inputs.
func addSignatures(txn *types.Transaction, cf types.CoveredFields, uc types.UnlockConditions, parentID crypto.Hash, spendKey spendableKey) error {
	return addSignaturesWithHashSigner(txn, cf, uc, parentID, spendKey, crypto.SignHash)
}

// addSignaturesWithHashSigner provides the same functionality as
// addSignatures, but allows the caller to specify the hash signing function.
func addSignaturesWithHashSigner(txn *types.Transaction, cf types.CoveredFields, uc types.UnlockConditions, parentID crypto.Hash, spendKey spendableKey, signHash hashSigner) error {
	// Try to find the matching secret key for each public key - some public
	// keys may not have a match. Some secret keys may be used multiple times,
	// which is why public keys are used as the outer loop.
	totalSignatures := uint64(0)
	for i, siaPubKey := range uc.PublicKeys {
		// Search for the matching secret key to the public key.
		for j := range spendKey.SecretKeys {
			pubKey := spendKey.SecretKeys[j].PublicKey()
			if bytes.Compare(siaPubKey.Key, pubKey[:]) != 0 {
				continue
			}

			// Found the right secret key, add a signature.
			sig := types.TransactionSignature{
				ParentID:       parentID,
				CoveredFields:  cf,
				PublicKeyIndex: uint64(i),
			}
			txn.TransactionSignatures = append(txn.TransactionSignatures, sig)
			sigIndex := len(txn.TransactionSignatures) - 1
			sigHash := txn.SigHash(sigIndex)
			encodedSig, err := signHash(sigHash, spendKey.SecretKeys[j])
			if err != nil {
				return err
			}
			txn.TransactionSignatures[sigIndex].Signature = encodedSig[:]

			// Count that the signature has been added, and break out of the
			// secret key loop.
			totalSignatures++
			break
		}

		// If there are enough signatures to satisfy the unlock conditions,
		// break out of the outer loop.
		if totalSignatures == uc.SignaturesRequired {
			break
		}
	}
	return nil
}
