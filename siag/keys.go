package main

// keys.go contains functions for generating and printing keys.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// The header for all siag files. Do not change.
	FileHeader = "siag"
)

var (
	ErrCorruptedKey       = errors.New("A corrupted key has been presented")
	ErrInsecureAddress    = errors.New("An address needs at least one required key to be secure")
	ErrUnspendableAddress = errors.New("An address is unspendable if the number of required keys is greater than the total number of keys")
)

// A KeyPair is the object that gets saved to disk for a signature key. All the
// information necessary to sign a transaction is in the struct, and the struct
// can be directly written to disk.
type KeyPair struct {
	Header           string
	Version          string
	Index            int
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
}

// generateKeys generates a set of keys and saves them to disk.
func generateKeys(requiredKeys int, totalKeys int, folder string, keyname string) (types.UnlockConditions, error) {
	// Check that the inputs have sane values.
	if requiredKeys < 1 {
		return types.UnlockConditions{}, ErrInsecureAddress
	}
	if totalKeys < requiredKeys {
		return types.UnlockConditions{}, ErrUnspendableAddress
	}

	// Generate 'TotalKeys', filling out everything except the unlock
	// conditions.
	keys := make([]KeyPair, totalKeys)
	pubKeys := make([]crypto.PublicKey, totalKeys)
	for i := range keys {
		var err error
		keys[i].Header = FileHeader
		keys[i].Version = Version
		keys[i].Index = i
		keys[i].SecretKey, pubKeys[i], err = crypto.GenerateSignatureKeys()
		if err != nil {
			return types.UnlockConditions{}, err
		}
	}

	// Generate the unlock conditions and add them to each KeyPair object. This
	// must be done second because the keypairs can't be given unlock
	// conditions until the PublicKeys have all been added.
	unlockConditions := types.UnlockConditions{
		Timelock:           0,
		SignaturesRequired: uint64(requiredKeys),
	}
	for i := range keys {
		unlockConditions.PublicKeys = append(unlockConditions.PublicKeys, types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       string(pubKeys[i][:]),
		})
	}
	for i := range keys {
		keys[i].UnlockConditions = unlockConditions
	}

	// Save the KeyPairs to disk.
	if folder != "" {
		err := os.MkdirAll(folder, 0700)
		if err != nil {
			return types.UnlockConditions{}, err
		}
	}
	for i, key := range keys {
		err := encoding.WriteFile(filepath.Join(folder, keyname+"_Key"+strconv.Itoa(i)+FileExtension), key)
		if err != nil {
			return types.UnlockConditions{}, err
		}
	}

	return unlockConditions, nil
}

// verifyKeys checks a set of keys on disk to see that they can spend funds
// sent to their address.
func verifyKeys(uc types.UnlockConditions, folder string, keyname string) error {
	keysRequired := uc.SignaturesRequired
	totalKeys := uint64(len(uc.PublicKeys))

	// Load the keys from disk back into memory, then verify that the keys on
	// disk are able to sign outputs in transactions.
	loadedKeys := make([]KeyPair, totalKeys)
	for i := 0; i < len(loadedKeys); i++ {
		err := encoding.ReadFile(filepath.Join(folder, keyname+"_Key"+strconv.Itoa(i)+FileExtension), &loadedKeys[i])
		if err != nil {
			return err
		}
	}

	// Check that the keys can be used to spend transactions.
	for _, loadedKey := range loadedKeys {
		if loadedKey.UnlockConditions.UnlockHash() != uc.UnlockHash() {
			return ErrCorruptedKey
		}
	}
	// Create a transaction for the keys to sign.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{
			types.SiafundInput{
				UnlockConditions: loadedKeys[0].UnlockConditions,
			},
		},
	}
	// Loop through and sign the transaction multiple times. All keys will be
	// used at least once by the time the loop terminates.
	var i uint64
	for i != totalKeys {
		// i tracks which key is next to be used. If i + RequiredKeys results
		// in going out-of-bounds, reduce i so that the last key will be used
		// for the final signature.
		if i+keysRequired > totalKeys {
			i = totalKeys - keysRequired
		}
		var j uint64
		for j < keysRequired {
			txn.TransactionSignatures = append(txn.TransactionSignatures, types.TransactionSignature{
				PublicKeyIndex: i,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			})
			sigHash := txn.SigHash(int(j))
			sig, err := crypto.SignHash(sigHash, loadedKeys[i].SecretKey)
			if err != nil {
				return err
			}
			txn.TransactionSignatures[j].Signature = types.Signature(sig[:])
			i++
			j++
		}
		// Check that the signature is valid.
		err := txn.StandaloneValid(0)
		if err != nil {
			return err
		}
		// Delete all of the signatures for the next iteration.
		txn.TransactionSignatures = nil
	}
	return nil
}

// generateKeys will generate a set of keys and save the keyfiles to disk.
func siag(*cobra.Command, []string) {
	unlockConditions, err := generateKeys(config.Siag.RequiredKeys, config.Siag.TotalKeys, config.Siag.Folder, config.Siag.AddressName)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = verifyKeys(unlockConditions, config.Siag.Folder, config.Siag.AddressName)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Keys created for address: %x\n", unlockConditions.UnlockHash())
	fmt.Printf("%v file(s) created. KEEP THESE FILES. To spend money from this address, you will need at least %v of the files.\n", config.Siag.TotalKeys, config.Siag.RequiredKeys)
}

// printKeyInfo opens a keyfile and prints the contents, returning an error if
// there's a problem.
func printKeyInfo(filename string) error {
	var kp KeyPair
	err := encoding.ReadFile(filename, &kp)
	if err != nil {
		return err
	}

	fmt.Printf("Found a key for a %v of %v address.\n", kp.UnlockConditions.SignaturesRequired, len(kp.UnlockConditions.PublicKeys))
	fmt.Printf("The address is: %x\n", kp.UnlockConditions.UnlockHash())
	return nil
}

// keyInfo receives the cobra call 'keyInfo', and is essentially a wrapper for
// printKeyInfo. This structure makes testing easier.
func keyInfo(c *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: siag keyinfo [filename]")
		return
	}
	err := printKeyInfo(args[0])
	if err != nil {
		fmt.Println(err)
		return
	}
}
