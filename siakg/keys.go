package main

// keys.go contains functions for generating and printing keys.

import (
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
	// The header for all siakg files. Do not change.
	FileHeader = "siakg"
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

// generateKeys will generate a set of keys and save the keyfiles to disk.
func generateKeys(*cobra.Command, []string) {
	// Check that the key requirements make sense.
	if config.Siakg.RequiredKeys == 0 {
		fmt.Println("An address with 0 required keys is not useful.")
		return
	}
	if config.Siakg.TotalKeys < config.Siakg.RequiredKeys {
		fmt.Printf("Total Keys (%v) must be greater than or equal to Required Keys (%v)\n", config.Siakg.TotalKeys, config.Siakg.RequiredKeys)
		return
	}

	fmt.Printf("Creating key '%s' with %v total keys and %v required keys.\n", config.Siakg.KeyName, config.Siakg.TotalKeys, config.Siakg.RequiredKeys)

	// Generate 'TotalKeys' keyparis and fill out the metadata.
	keys := make([]KeyPair, config.Siakg.TotalKeys)
	for i := range keys {
		keys[i].Header = FileHeader
		keys[i].Version = Version
		keys[i].Index = i
	}

	// Add the keys to each keypair.
	pubKeys := make([]crypto.PublicKey, config.Siakg.TotalKeys)
	for i := 0; i < config.Siakg.TotalKeys; i++ {
		var err error
		keys[i].SecretKey, pubKeys[i], err = crypto.GenerateSignatureKeys()
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	// Generate the unlock conditions and add them to each KeyPair object.
	uc := types.UnlockConditions{
		Timelock:           0,
		RequiredSignatures: uint64(config.Siakg.RequiredKeys),
	}
	for i := range keys {
		uc.PublicKeys = append(uc.PublicKeys, types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       string(pubKeys[i][:]),
		})
	}
	for i := range keys {
		keys[i].UnlockConditions = uc
	}

	// Save the KeyPairs to disk.
	if config.Siakg.Folder != "" {
		err := os.MkdirAll(config.Siakg.Folder, 0700)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	for i, key := range keys {
		err := encoding.WriteFile(filepath.Join(config.Siakg.Folder, config.Siakg.KeyName)+"_Key"+strconv.Itoa(i)+FileExtension, key)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	// Load the keys from disk back into memory, then verifiy that the keys on
	// disk are able to sign outputs in transactions.
	loadedKeys := make([]KeyPair, config.Siakg.TotalKeys)
	for i := 0; i < len(loadedKeys); i++ {
		err := encoding.ReadFile(filepath.Join(config.Siakg.Folder, config.Siakg.KeyName)+"_Key"+strconv.Itoa(i)+FileExtension, &loadedKeys[i])
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	// Check that the keys can be used to spend transactions. Load them back into memory.
	for _, loadedKey := range loadedKeys {
		if loadedKey.UnlockConditions.UnlockHash() != uc.UnlockHash() {
			fmt.Println("corruption occured while saving the keys to disk")
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
	var i int
	for i != config.Siakg.TotalKeys {
		// i tracks which key is next to be used. If i + RequiredKeys results
		// in going out-of-bounds, reduce i so that the last key will be used
		// for the final signature.
		if i+config.Siakg.RequiredKeys > config.Siakg.TotalKeys {
			i = config.Siakg.TotalKeys - config.Siakg.RequiredKeys
		}
		var j int
		for j < config.Siakg.RequiredKeys {
			txn.TransactionSignatures = append(txn.TransactionSignatures, types.TransactionSignature{
				PublicKeyIndex: uint64(i),
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			})
			sigHash := txn.SigHash(j)
			sig, err := crypto.SignHash(sigHash, loadedKeys[i].SecretKey)
			if err != nil {
				fmt.Println(err)
				return
			}
			txn.TransactionSignatures[j].Signature = types.Signature(sig[:])
			i++
			j++
		}
		// Check that the signature is valid.
		err := txn.StandaloneValid(0)
		if err != nil {
			fmt.Println(err)
		}
		// Delete all of the signatures for the next iteration.
		txn.TransactionSignatures = nil
	}

	fmt.Printf("Success, the address for this set of keys is: %x\n", uc.UnlockHash())
}

// printKey opens a keyfile and prints the contents.
func printKey(*cobra.Command, []string) {
	var kp KeyPair
	err := encoding.ReadFile(config.Address.Filename, &kp)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Found a key for a %v of %v address.\n", kp.UnlockConditions.RequiredSignatures, len(kp.UnlockConditions.PublicKeys))
	fmt.Printf("The address is: %x\n", kp.UnlockConditions.UnlockHash())
}
