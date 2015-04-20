package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules/tester"
)

// TestGenerateKeys probes the generateKeys function.
func TestGenerateKeys(t *testing.T) {
	testDir := tester.TempDir("siakg", "TestGenerateKeys")

	// Try to create an anyone-can-spend set of keys.
	_, err := generateKeys(0, 0, testDir, "anyoneCanSpend")
	if err != ErrInsecureAddress {
		t.Error("Expecting ErrInsecureAddress:", err)
	}
	// Try to create an unspendable address.
	_, err = generateKeys(1, 0, testDir, "unspendable")
	if err != ErrUnspendableAddress {
		t.Error("Expecting ErrUnspendableAddress:", err)
	}

	// Create a legitimate set of keys.
	_, err = generateKeys(1, 1, testDir, "genuine")
	if err != nil {
		t.Error(err)
	}
	// Check that the file was created.
	_, err = os.Stat(filepath.Join(testDir, "genuine_Key0"+FileExtension))
	if err != nil {
		t.Error(err)
	}
}

// TestVerifyKeys proves the verifyKeys function.
func TestVerifyKeys(t *testing.T) {
	testDir := tester.TempDir("siakg", "TestVerifyKeys")

	// Create sets of keys that cover all boundaries from 0 of 1 to 5 of 9.
	// This is to check for errors in the keycheck calculations.
	for i := 1; i < 5; i++ {
		for j := i; j < 9; j++ {
			keyname := "genuine" + strconv.Itoa(i) + strconv.Itoa(j)
			uc, err := generateKeys(i, j, testDir, keyname)
			if err != nil {
				t.Error(err)
			}

			// Check that the validate under standard conditions.
			err = verifyKeys(uc, testDir, keyname)
			if err != nil {
				t.Error(err)
			}

			// Provide the wrong keyname to simulate a file does not exist error.
			err = verifyKeys(uc, testDir, "wrongName")
			if err == nil {
				t.Error("Expecting an error")
			}

			// Corrupt the unlock conditions of the files 1 by 1, and see that each
			// file is checked for validity.
			for k := 0; k < j; k++ {
				// Load, corrupt, and then save the keypair. This corruption
				// alters the UnlockConditions.
				var originalKP, badKP KeyPair
				keyfile := filepath.Join(testDir, keyname+"_Key"+strconv.Itoa(k)+FileExtension)
				err := encoding.ReadFile(keyfile, &originalKP)
				if err != nil {
					t.Fatal(err)
				}
				badKP = originalKP
				badKP.UnlockConditions.PublicKeys = nil
				err = encoding.WriteFile(keyfile, badKP)
				if err != nil {
					t.Fatal(err)
				}

				// Run verifyKeys with the corrupted file.
				err = verifyKeys(uc, testDir, keyname)
				if err == nil {
					t.Error("Expecting error after corrupting unlock conditions")
				}

				// Restore the original keyfile.
				err = encoding.WriteFile(keyfile, originalKP)
				if err != nil {
					t.Fatal(err)
				}

				// Verify that things work again.
				err = verifyKeys(uc, testDir, keyname)
				if err != nil {
					t.Fatal(err)
				}
			}

			// Corrupt the secret keys of the files 1 by 1, and see that each secret
			// key is checked for validity.
			for k := 0; k < j; k++ {
				// Load, corrupt, and then save the keypair. This corruption
				// alters the secret key.
				var originalKP, badKP KeyPair
				keyfile := filepath.Join(testDir, keyname+"_Key"+strconv.Itoa(k)+FileExtension)
				err := encoding.ReadFile(keyfile, &originalKP)
				if err != nil {
					t.Fatal(err)
				}
				badKP = originalKP
				badKP.SecretKey[0]++
				err = encoding.WriteFile(keyfile, badKP)
				if err != nil {
					t.Fatal(err)
				}

				// Run verifyKeys with the corrupted file.
				err = verifyKeys(uc, testDir, keyname)
				if err == nil {
					t.Error("Expecting error after corrupting unlock conditions")
				}

				// Restore the original keyfile.
				err = encoding.WriteFile(keyfile, originalKP)
				if err != nil {
					t.Fatal(err)
				}

				// Verify that things work again.
				err = verifyKeys(uc, testDir, keyname)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
