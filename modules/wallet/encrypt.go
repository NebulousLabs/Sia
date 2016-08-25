package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
)

var (
	errAlreadyUnlocked   = errors.New("wallet has already been unlocked")
	errReencrypt         = errors.New("wallet is already encrypted, cannot encrypt again")
	errUnencryptedWallet = errors.New("wallet has not been encrypted yet")

	// verificationPlaintext is the plaintext used to verify encryption keys.
	// By storing the corresponding ciphertext for a given key, we can later
	// verify that a key is correct by using it to decrypt the ciphertext and
	// comparing the result to verificationPlaintext.
	verificationPlaintext = make([]byte, 32)
)

// uidEncryptionKey creates an encryption key that is used to decrypt a
// specific key file.
func uidEncryptionKey(masterKey crypto.TwofishKey, uid uniqueID) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, uid))
}

// verifyEncryption verifies that key properly decrypts the ciphertext to a
// preset plaintext.
func verifyEncryption(key crypto.TwofishKey, encrypted crypto.Ciphertext) error {
	verification, err := key.DecryptBytes(encrypted)
	if err != nil {
		return modules.ErrBadEncryptionKey
	}
	if !bytes.Equal(verificationPlaintext, verification) {
		return modules.ErrBadEncryptionKey
	}
	return nil
}

// checkMasterKey verifies that the masterKey is the key used to encrypt the wallet.
func checkMasterKey(tx *bolt.Tx, masterKey crypto.TwofishKey) error {
	uk := uidEncryptionKey(masterKey, dbGetWalletUID(tx))
	encryptedVerification := tx.Bucket(bucketWallet).Get(keyEncryptionVerification)
	return verifyEncryption(uk, encryptedVerification)
}

// initEncryption checks that the provided encryption key is the valid
// encryption key for the wallet. If encryption has not yet been established
// for the wallet, an encryption key is created.
func (w *Wallet) initEncryption(masterKey crypto.TwofishKey) (modules.Seed, error) {
	// Create a random seed.
	var seed modules.Seed
	_, err := rand.Read(seed[:])
	if err != nil {
		return modules.Seed{}, err
	}

	err = w.db.Update(func(tx *bolt.Tx) error {
		wb := tx.Bucket(bucketWallet)
		// Check if the wallet encryption key has already been set.
		if wb.Get(keyEncryptionVerification) != nil {
			return errReencrypt
		}

		// If the input key is blank, use the hash of the seed to create the
		// master key. Otherwise, use the input key.
		if masterKey == (crypto.TwofishKey{}) {
			masterKey = crypto.TwofishKey(crypto.HashObject(seed))
		}

		// create a seedFile for the seed
		sf, err := createSeedFile(masterKey, seed)
		if err != nil {
			return err
		}

		// set this as the primary seedFile
		err = wb.Put(keyPrimarySeedFile, encoding.Marshal(sf))
		if err != nil {
			return err
		}
		err = wb.Put(keyPrimarySeedProgress, encoding.Marshal(uint64(0)))
		if err != nil {
			return err
		}

		// Establish the encryption verification using the masterKey. After this
		// point, the wallet is encrypted.
		uk := uidEncryptionKey(masterKey, dbGetWalletUID(tx))
		verification, err := uk.EncryptBytes(verificationPlaintext)
		if err != nil {
			return err
		}
		err = wb.Put(keyEncryptionVerification, verification[:])
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return modules.Seed{}, err
	}

	// on future startups, this field will be set by w.initPersist
	w.encrypted = true

	return seed, nil
}

// managedUnlock loads all of the encrypted file structures into wallet memory. Even
// after loading, the structures are kept encrypted, but some data such as
// addresses are decrypted so that the wallet knows what to track.
func (w *Wallet) managedUnlock(masterKey crypto.TwofishKey) error {
	w.mu.RLock()
	unlocked := w.unlocked
	encrypted := w.encrypted
	w.mu.RUnlock()
	if unlocked {
		return errAlreadyUnlocked
	} else if !encrypted {
		return errUnencryptedWallet
	}

	// Load db objects into memory.
	var lastChange modules.ConsensusChangeID
	var primarySeedFile seedFile
	var primarySeedProgress uint64
	var auxiliarySeedFiles []seedFile
	var unseededKeyFiles []spendableKeyFile
	err := w.db.View(func(tx *bolt.Tx) error {
		// verify masterKey
		err := checkMasterKey(tx, masterKey)
		if err != nil {
			return err
		}

		// lastChange
		lastChange = dbGetConsensusChangeID(tx)

		// primarySeedFile + primarySeedProgress
		err = encoding.Unmarshal(tx.Bucket(bucketWallet).Get(keyPrimarySeedFile), &primarySeedFile)
		if err != nil {
			return err
		}
		err = encoding.Unmarshal(tx.Bucket(bucketWallet).Get(keyPrimarySeedProgress), &primarySeedProgress)
		if err != nil {
			return err
		}

		// auxiliarySeedFiles
		err = tx.Bucket(bucketSeedFiles).ForEach(func(_, sfBytes []byte) error {
			var sf seedFile
			err := encoding.Unmarshal(sfBytes, &sf)
			if err != nil {
				return err
			}
			auxiliarySeedFiles = append(auxiliarySeedFiles, sf)
			return nil
		})
		if err != nil {
			return err
		}

		// unseededKeyFiles
		err = tx.Bucket(bucketSpendableKeyFiles).ForEach(func(_, ukfBytes []byte) error {
			var ukf spendableKeyFile
			err := encoding.Unmarshal(ukfBytes, &ukf)
			if err != nil {
				return err
			}
			unseededKeyFiles = append(unseededKeyFiles, ukf)
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Decrypt + load keys.
	err = func() error {
		w.mu.Lock()
		defer w.mu.Unlock()

		// primarySeedFile
		primarySeed, err := decryptSeedFile(masterKey, primarySeedFile)
		if err != nil {
			return err
		}
		w.integrateSeed(primarySeed, primarySeedProgress)
		w.primarySeed = primarySeed

		// auxiliarySeedFiles
		for _, sf := range auxiliarySeedFiles {
			auxSeed, err := decryptSeedFile(masterKey, sf)
			if err != nil {
				return err
			}
			w.integrateSeed(auxSeed, modules.PublicKeysPerSeed)
			w.seeds = append(w.seeds, auxSeed)
		}

		// unseededKeyFiles
		for _, uk := range unseededKeyFiles {
			sk, err := decryptSpendableKeyFile(masterKey, uk)
			if err != nil {
				return err
			}
			w.integrateSpendableKey(masterKey, sk)
		}
		return nil
	}()
	if err != nil {
		return err
	}

	// Subscribe to the consensus set if this is the first unlock for the
	// wallet object.
	w.mu.RLock()
	subscribed := w.subscribed
	w.mu.RUnlock()
	if !subscribed {
		err = w.cs.ConsensusSetSubscribe(w, lastChange)
		if err == modules.ErrInvalidConsensusChangeID {
			// something went wrong; resubscribe from the beginning and spawn a
			// goroutine to display rescan progress
			if build.Release != "testing" {
				go func() {
					println("Rescanning consensus set...")
					for range time.Tick(time.Second * 3) {
						w.mu.RLock()
						var height types.BlockHeight
						w.db.View(func(tx *bolt.Tx) error {
							var err error
							height, err = dbGetConsensusHeight(tx)
							return err
						})
						done := w.subscribed
						w.mu.RUnlock()
						if done {
							println("\nDone!")
							break
						}
						print("\rScanned to height ", height, "...")
					}
				}()
			}
			// set the db entries for consensus change and consensus height
			err = w.db.Update(func(tx *bolt.Tx) error {
				err := dbPutConsensusChangeID(tx, modules.ConsensusChangeBeginning)
				if err != nil {
					return err
				}
				return dbPutConsensusHeight(tx, 0)
			})
			if err != nil {
				return errors.New("failed to reset db during rescan: " + err.Error())
			}
			err = w.cs.ConsensusSetSubscribe(w, modules.ConsensusChangeBeginning)
		}
		if err != nil {
			return errors.New("wallet subscription failed: " + err.Error())
		}
		w.tpool.TransactionPoolSubscribe(w)
		w.mu.Lock()
		w.subscribed = true
		w.mu.Unlock()
	}

	w.mu.Lock()
	w.unlocked = true
	w.mu.Unlock()
	return nil
}

// wipeSecrets erases all of the seeds and secret keys in the wallet.
func (w *Wallet) wipeSecrets() {
	// 'for i := range' must be used to prevent copies of secret data from
	// being made.
	for i := range w.keys {
		for j := range w.keys[i].SecretKeys {
			crypto.SecureWipe(w.keys[i].SecretKeys[j][:])
		}
	}
	for i := range w.seeds {
		crypto.SecureWipe(w.seeds[i][:])
	}
	crypto.SecureWipe(w.primarySeed[:])
	w.seeds = w.seeds[:0]
}

// Encrypted returns whether or not the wallet has been encrypted.
func (w *Wallet) Encrypted() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if build.DEBUG && w.unlocked && !w.encrypted {
		panic("wallet is both unlocked and unencrypted")
	}
	return w.encrypted
}

// Encrypt will encrypt the wallet using the input key. Upon encryption, a
// primary seed will be created for the wallet (no seed exists prior to this
// point). If the key is blank, then the hash of the seed that is generated
// will be used as the key. The wallet will still be locked after encryption.
//
// Encrypt can only be called once throughout the life of the wallet, and will
// return an error on subsequent calls (even after restarting the wallet). To
// reset the wallet, the wallet files must be moved to a different directory or
// deleted.
func (w *Wallet) Encrypt(masterKey crypto.TwofishKey) (modules.Seed, error) {
	if err := w.tg.Add(); err != nil {
		return modules.Seed{}, err
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.initEncryption(masterKey)
}

// Unlocked indicates whether the wallet is locked or unlocked.
func (w *Wallet) Unlocked() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.unlocked
}

// Lock will erase all keys from memory and prevent the wallet from spending
// coins until it is unlocked.
func (w *Wallet) Lock() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return modules.ErrLockedWallet
	}
	w.log.Println("INFO: Locking wallet.")

	// Wipe all of the seeds and secret keys. They will be replaced upon
	// calling 'Unlock' again. Note that since the public keys are not wiped,
	// we can continue processing blocks.
	w.wipeSecrets()
	w.unlocked = false
	return nil
}

// Unlock will decrypt the wallet seed and load all of the addresses into
// memory.
func (w *Wallet) Unlock(masterKey crypto.TwofishKey) error {
	// By having the wallet's ThreadGroup track the Unlock method, we ensure
	// that Unlock will never unlock the wallet once the ThreadGroup has been
	// stopped. Without this precaution, the wallet's Close method would be
	// unsafe because it would theoretically be possible for another function
	// to Unlock the wallet in the short interval after Close calls w.Lock
	// and before Close calls w.mu.Lock.
	if err := w.tg.Add(); err != nil {
		return err
	}
	defer w.tg.Done()
	w.log.Println("INFO: Unlocking wallet.")

	// Initialize all of the keys in the wallet under a lock. While holding the
	// lock, also grab the subscriber status.
	return w.managedUnlock(masterKey)
}
