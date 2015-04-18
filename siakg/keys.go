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

type KeyPair struct {
	Index            int
	SecretKey        crypto.SecretKey
	PublicKey        crypto.PublicKey
	UnlockConditions types.UnlockConditions
}

func generateKeys(*cobra.Command, []string) {
	fmt.Printf("Creating key '%s' with %v total keys and %v required keys.\n", config.Siakg.KeyName, config.Siakg.TotalKeys, config.Siakg.RequiredKeys)

	// Generate 'TotalKeys' keyparis.
	keys := make([]KeyPair, config.Siakg.TotalKeys)
	for i := 0; i < config.Siakg.TotalKeys; i++ {
		var err error
		keys[i].SecretKey, keys[i].PublicKey, err = crypto.GenerateSignatureKeys()
		if err != nil {
			fmt.Println(err)
			return
		}
		keys[i].Index = i
	}

	// Generate the 'UnlockConditions'.
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
