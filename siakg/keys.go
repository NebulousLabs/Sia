package main

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
	FILEHEADER = "siakg"
)

// A KeyPair is the object that gets saved to disk for a signature key. All the
// information necessary to sign a transaction is in the struct, and the struct
// can be directly written to disk.
type KeyPair struct {
	Header           string
	Version          string
	Index            int
	SecretKey        crypto.SecretKey
	PublicKey        crypto.PublicKey
	UnlockConditions types.UnlockConditions
}

// generateKeys will generate a set of keys and save the keyfiles to disk.
func generateKeys(*cobra.Command, []string) {
	// Check that the total number of keys is at least as large as the required
	// number of keys.
	if config.Siakg.TotalKeys < config.Siakg.RequiredKeys {
		fmt.Printf("Total Keys (%v) must be greater than or equal to Required Keys (%v)\n", config.Siakg.TotalKeys, config.Siakg.RequiredKeys)
		return
	}

	fmt.Printf("Creating key '%s' with %v total keys and %v required keys.\n", config.Siakg.KeyName, config.Siakg.TotalKeys, config.Siakg.RequiredKeys)

	// Generate 'TotalKeys' keyparis and fill out the metadata.
	keys := make([]KeyPair, config.Siakg.TotalKeys)
	for i := range keys {
		keys[i].Header = FILEHEADER
		keys[i].Version = VERSION
		keys[i].Index = i
	}

	// Add the keys to each keypair.
	for i := 0; i < config.Siakg.TotalKeys; i++ {
		var err error
		keys[i].SecretKey, keys[i].PublicKey, err = crypto.GenerateSignatureKeys()
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	// Generate the unlock conditions and add them to each KeyPair object.
	uc := types.UnlockConditions{
		Timelock:      0,
		NumSignatures: uint64(config.Siakg.RequiredKeys),
	}
	for i := range keys {
		uc.PublicKeys = append(uc.PublicKeys, types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       string(keys[i].PublicKey[:]),
		})
	}
	for i := range keys {
		keys[i].UnlockConditions = uc
	}

	// Save the KeyPairs to disk.
	err := os.MkdirAll(config.Siakg.KeyName, 0700)
	if err != nil {
		fmt.Println(err)
		return
	}
	for i, key := range keys {
		err = encoding.WriteFile(filepath.Join(config.Siakg.KeyName, config.Siakg.KeyName+"_Key"+strconv.Itoa(i)), key)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	fmt.Printf("Success, the address for this set of keys is: %x\n", uc.UnlockHash())
}
